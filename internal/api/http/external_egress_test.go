package http

import (
	"context"
	"encoding/json"
	"errors"
	nethttp "net/http"
	"net/http/httptest"
	"testing"

	"github.com/rtis-emc2/megavpn/internal/domain"
)

type externalEgressProfilePageTestStore struct {
	Store
	page                             domain.ExternalEgressProfilePage
	search, protocol, status, health string
	limit, offset                    int
}

func (s *externalEgressProfilePageTestStore) ListExternalEgressProfilePage(_ context.Context, search, protocol, status, health string, limit, offset int) (domain.ExternalEgressProfilePage, error) {
	s.search, s.protocol, s.status, s.health, s.limit, s.offset = search, protocol, status, health, limit, offset
	return s.page, nil
}

func TestListExternalEgressProfilePage(t *testing.T) {
	store := &externalEgressProfilePageTestStore{page: domain.ExternalEgressProfilePage{
		Items: []domain.ExternalEgressProfile{{ID: "profile-1", DisplayName: "Dallas"}}, Total: 2500, Limit: 100, Offset: 200,
	}}
	server := &Server{store: store}
	request := httptest.NewRequest(nethttp.MethodGet, "/api/v1/external-egress/profiles:page?search=dallas&protocol=openvpn&status=active&health=attention&limit=999&offset=200", nil)
	response := httptest.NewRecorder()

	server.listExternalEgressProfilePage(response, request)

	if response.Code != nethttp.StatusOK {
		t.Fatalf("status = %d, want %d: %s", response.Code, nethttp.StatusOK, response.Body.String())
	}
	if store.search != "dallas" || store.protocol != "openvpn" || store.status != "active" || store.health != "attention" || store.limit != 100 || store.offset != 200 {
		t.Fatalf("unexpected filters: search=%q protocol=%q status=%q health=%q limit=%d offset=%d", store.search, store.protocol, store.status, store.health, store.limit, store.offset)
	}
	var page domain.ExternalEgressProfilePage
	if err := json.Unmarshal(response.Body.Bytes(), &page); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if page.Total != 2500 || len(page.Items) != 1 {
		t.Fatalf("unexpected page response: %+v", page)
	}
}

func TestListExternalEgressProfilePageRejectsInvalidHealth(t *testing.T) {
	server := &Server{store: &externalEgressProfilePageTestStore{}}
	request := httptest.NewRequest(nethttp.MethodGet, "/api/v1/external-egress/profiles:page?health=broken", nil)
	response := httptest.NewRecorder()

	server.listExternalEgressProfilePage(response, request)

	if response.Code != nethttp.StatusBadRequest {
		t.Fatalf("status = %d, want %d: %s", response.Code, nethttp.StatusBadRequest, response.Body.String())
	}
}

func TestListExternalEgressProfilePageRejectsInvalidStatus(t *testing.T) {
	server := &Server{store: &externalEgressProfilePageTestStore{}}
	request := httptest.NewRequest(nethttp.MethodGet, "/api/v1/external-egress/profiles:page?status=deleted", nil)
	response := httptest.NewRecorder()

	server.listExternalEgressProfilePage(response, request)

	if response.Code != nethttp.StatusBadRequest {
		t.Fatalf("status = %d, want %d: %s", response.Code, nethttp.StatusBadRequest, response.Body.String())
	}
}

type externalEgressDeploymentDeleteTestStore struct {
	Store
	deployment domain.ExternalEgressDeployment
	err        error
	gotID      string
}

func (s *externalEgressDeploymentDeleteTestStore) DeleteExternalEgressDeployment(_ context.Context, deploymentID string, _ *string) (domain.ExternalEgressDeployment, error) {
	s.gotID = deploymentID
	return s.deployment, s.err
}

func TestDeleteExternalEgressDeployment(t *testing.T) {
	store := &externalEgressDeploymentDeleteTestStore{
		deployment: domain.ExternalEgressDeployment{
			ID: "deployment-1", DesiredStatus: "deleted", Status: "deleted",
		},
	}
	server := &Server{store: store}
	request := httptest.NewRequest(nethttp.MethodDelete, "/api/v1/external-egress/deployments/deployment-1", nil)
	request.SetPathValue("deployment_id", " deployment-1 ")
	response := httptest.NewRecorder()

	server.deleteExternalEgressDeployment(response, request)

	if response.Code != nethttp.StatusOK {
		t.Fatalf("status = %d, want %d: %s", response.Code, nethttp.StatusOK, response.Body.String())
	}
	if store.gotID != "deployment-1" {
		t.Fatalf("deployment id = %q, want deployment-1", store.gotID)
	}
}

func TestDeleteExternalEgressDeploymentMapsLifecycleConflict(t *testing.T) {
	store := &externalEgressDeploymentDeleteTestStore{err: errors.New("cleanup the external egress deployment before removing it from the node")}
	server := &Server{store: store}
	request := httptest.NewRequest(nethttp.MethodDelete, "/api/v1/external-egress/deployments/deployment-1", nil)
	request.SetPathValue("deployment_id", "deployment-1")
	response := httptest.NewRecorder()

	server.deleteExternalEgressDeployment(response, request)

	if response.Code != nethttp.StatusConflict {
		t.Fatalf("status = %d, want %d: %s", response.Code, nethttp.StatusConflict, response.Body.String())
	}
}
