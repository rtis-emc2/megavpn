package postgres

import (
	"context"
	"fmt"
	"reflect"
	"strings"

	"github.com/rtis-emc2/megavpn/internal/domain"
)

const backhaulXrayConvergenceSource = "system:backhaul-transport-change"

func (s *Store) queueBackhaulRouteRefreshOrXrayConvergence(ctx context.Context, link domain.BackhaulLink, source string) ([]domain.Job, error) {
	if !strings.EqualFold(strings.TrimSpace(link.Status), "active") {
		return nil, nil
	}
	xrayJobs, err := s.queueXrayRemoteEgressConvergenceForBackhaul(ctx, link, source)
	if err != nil {
		return nil, err
	}
	if len(xrayJobs) > 0 {
		return xrayJobs, nil
	}
	routeJob, err := s.CreateNodeRoutePolicyApplyJob(ctx, link.IngressNodeID)
	if err != nil {
		return nil, err
	}
	return []domain.Job{routeJob}, nil
}

func (s *Store) queueXrayRemoteEgressConvergenceForBackhaul(ctx context.Context, link domain.BackhaulLink, source string) ([]domain.Job, error) {
	source = strings.TrimSpace(source)
	if source == "" {
		source = backhaulXrayConvergenceSource
	}
	if strings.TrimSpace(link.IngressNodeID) == "" || strings.TrimSpace(link.EgressNodeID) == "" {
		return nil, nil
	}
	selected := selectedBackhaulTransport(link)
	if selected == nil || !strings.EqualFold(strings.TrimSpace(selected.Status), "active") {
		return nil, nil
	}
	instances, err := s.listXrayVLESSGroupSyncInstances(ctx)
	if err != nil {
		return nil, err
	}
	jobs := make([]domain.Job, 0)
	for _, instance := range instances {
		if strings.TrimSpace(instance.NodeID) != strings.TrimSpace(link.IngressNodeID) {
			continue
		}
		if !xrayInstanceSpecReferencesBackhaulEgress(instance.Spec, link) {
			continue
		}
		job, queued, err := s.refreshXrayRemoteEgressForBackhaul(ctx, instance, link, source)
		if err != nil {
			return jobs, err
		}
		if queued {
			jobs = append(jobs, job)
		}
	}
	if len(jobs) > 0 {
		_, _ = s.CreateAudit(ctx, "system", "backhaul.xray_converge", "backhaul", &link.ID, fmt.Sprintf("queued %d xray apply job(s) before route policy refresh", len(jobs)))
	}
	return jobs, nil
}

func (s *Store) refreshXrayRemoteEgressForBackhaul(ctx context.Context, instance domain.Instance, link domain.BackhaulLink, source string) (domain.Job, bool, error) {
	original := cloneMap(instance.Spec)
	next := cloneMap(instance.Spec)
	seedXrayBackhaulEgressNodeID(next, link)
	if _, err := s.resolveXrayDefaultEgress(ctx, instance, next, source); err != nil {
		return domain.Job{}, false, fmt.Errorf("refresh xray default egress for instance %s: %w", instance.ID, err)
	}
	if _, err := resolveXrayVLESSGroupEgressWithResolver(ctx, instance, next, s.ResolveXrayVLESSEgress, source); err != nil {
		return domain.Job{}, false, fmt.Errorf("refresh xray vless group egress for instance %s: %w", instance.ID, err)
	}
	if reflect.DeepEqual(original, next) {
		return domain.Job{}, false, nil
	}
	materialized, err := s.materializeInstanceDriverSpecDefaults(ctx, instance, next)
	if err != nil {
		return domain.Job{}, false, fmt.Errorf("materialize xray egress convergence for instance %s: %w", instance.ID, err)
	}
	status, _, validationErrors := s.validateInstanceRevisionSpec(ctx, instance, materialized)
	if !in(status, "validated", "applied") {
		return domain.Job{}, false, fmt.Errorf("xray egress convergence revision is not apply-ready for instance %s; status=%s errors=%v", instance.ID, status, validationErrors)
	}
	revision, err := s.ReplaceInstanceSpec(ctx, instance.ID, source, materialized)
	if err != nil {
		return domain.Job{}, false, fmt.Errorf("replace xray egress convergence revision for instance %s: %w", instance.ID, err)
	}
	if !in(strings.TrimSpace(revision.Status), "validated", "applied") {
		return domain.Job{}, false, fmt.Errorf("xray egress convergence revision is not apply-ready for instance %s; status=%s errors=%v", instance.ID, strings.TrimSpace(revision.Status), revision.ValidationErrors)
	}
	if ok, reason := shouldQueueVLESSGroupCatalogApply(instance); !ok {
		_, _ = s.CreateAudit(ctx, "system", "backhaul.xray_converge.skip", "instance", &instance.ID, "xray egress convergence revision created without apply: "+reason)
		return domain.Job{}, false, nil
	}
	job, err := s.UpdateInstanceStatus(ctx, instance.ID, "apply")
	if err != nil {
		return domain.Job{}, false, fmt.Errorf("queue xray apply after egress convergence for instance %s: %w", instance.ID, err)
	}
	_, _ = s.CreateAudit(ctx, "system", "backhaul.xray_converge.apply", "instance", &instance.ID, "xray apply queued after backhaul transport change")
	return job, true, nil
}

func xrayInstanceSpecReferencesBackhaulEgress(spec map[string]any, link domain.BackhaulLink) bool {
	if spec == nil {
		return false
	}
	if xrayEgressReferenceMatchesBackhaul(spec, link) {
		return true
	}
	if xrayEgressReferenceMatchesBackhaul(mapFromAny(spec["xray_egress"]), link) {
		return true
	}
	for _, raw := range xraySpecGroupItems(spec) {
		group := mapFromAny(raw)
		if len(group) == 0 {
			continue
		}
		if xrayEgressReferenceMatchesBackhaul(group, link) {
			return true
		}
		if xrayEgressReferenceMatchesBackhaul(mapFromAny(group["egress"]), link) {
			return true
		}
	}
	return false
}

func seedXrayBackhaulEgressNodeID(spec map[string]any, link domain.BackhaulLink) bool {
	if spec == nil || strings.TrimSpace(link.EgressNodeID) == "" {
		return false
	}
	changed := false
	if seedXrayBackhaulEgressNodeIDInMap(spec, link) {
		changed = true
	}
	if egress := mapFromAny(spec["xray_egress"]); len(egress) > 0 {
		if seedXrayBackhaulEgressNodeIDInMap(egress, link) {
			spec["xray_egress"] = egress
			changed = true
		}
	}
	groupsKey, groups := xrayMutableSpecGroupItems(spec)
	if len(groups) == 0 {
		return changed
	}
	updated := make([]any, 0, len(groups))
	groupsChanged := false
	for _, raw := range groups {
		group, _ := cloneAny(raw).(map[string]any)
		if group == nil {
			updated = append(updated, raw)
			continue
		}
		groupChanged := false
		if seedXrayBackhaulEgressNodeIDInMap(group, link) {
			groupChanged = true
		}
		if egress := mapFromAny(group["egress"]); len(egress) > 0 {
			if seedXrayBackhaulEgressNodeIDInMap(egress, link) {
				group["egress"] = egress
				groupChanged = true
			}
		}
		if groupChanged {
			groupsChanged = true
			updated = append(updated, group)
		} else {
			updated = append(updated, raw)
		}
	}
	if groupsChanged {
		spec[groupsKey] = updated
		changed = true
	}
	return changed
}

func seedXrayBackhaulEgressNodeIDInMap(ref map[string]any, link domain.BackhaulLink) bool {
	if !xrayEgressReferenceMatchesBackhaul(ref, link) {
		return false
	}
	if firstString(ref["egress_node_id"], ref["xray_egress_node_id"], ref["vless_egress_node_id"], ref["node_id"]) != "" {
		return false
	}
	ref["egress_node_id"] = strings.TrimSpace(link.EgressNodeID)
	return true
}

func xrayEgressReferenceMatchesBackhaul(ref map[string]any, link domain.BackhaulLink) bool {
	if len(ref) == 0 {
		return false
	}
	egressNodeID := firstString(ref["egress_node_id"], ref["xray_egress_node_id"], ref["vless_egress_node_id"], ref["node_id"])
	if egressNodeID != "" && egressNodeID == strings.TrimSpace(link.EgressNodeID) {
		return true
	}
	linkID := firstString(ref["link_id"], ref["backhaul_link_id"])
	if linkID != "" && linkID == strings.TrimSpace(link.ID) {
		return true
	}
	transportID := firstString(ref["transport_id"], ref["backhaul_transport_id"])
	if transportID == "" {
		return false
	}
	for _, transport := range link.Transports {
		if transportID == strings.TrimSpace(transport.ID) {
			return true
		}
	}
	return false
}
