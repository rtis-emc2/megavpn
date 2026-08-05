package http

import (
	"context"
	"errors"
	nethttp "net/http"
	"net/http/httptest"
	"testing"

	"github.com/rtis-emc2/megavpn/internal/domain"
)

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
