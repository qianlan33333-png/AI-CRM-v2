package platformhttp

import (
	"context"

	"github.com/qianlan33333-png/AI-CRM-v2/internal/api/generated"
)

type HealthHandler struct{}

var _ generated.StrictServerInterface = (*HealthHandler)(nil)

func NewHealthHandler() *HealthHandler {
	return &HealthHandler{}
}

func (*HealthHandler) GetHealthz(context.Context, generated.GetHealthzRequestObject) (generated.GetHealthzResponseObject, error) {
	return generated.GetHealthz200JSONResponse{Status: generated.Ok}, nil
}
