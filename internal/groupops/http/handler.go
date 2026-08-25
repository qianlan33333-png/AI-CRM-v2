// Package groupopshttp exposes the local-only Group Ops HTTP contract.
package groupopshttp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"

	authport "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/port"
	groupopsapp "github.com/qianlan33333-png/AI-CRM-v2/internal/groupops/app"
	groupopsport "github.com/qianlan33333-png/AI-CRM-v2/internal/groupops/port"
)

const (
	PlansPath             = "/api/admin/automation-conversion/group-ops/plans"
	PlanPath              = PlansPath + "/{plan_id}"
	PlanActivatePath      = PlanPath + "/activate"
	PlanPausePath         = PlanPath + "/pause"
	PlanArchivePath       = PlanPath + "/archive"
	MembersPath           = PlanPath + "/members"
	MemberPath            = MembersPath + "/{staff_id}"
	GroupAssetsPath       = PlanPath + "/group-assets"
	GroupAssetPath        = GroupAssetsPath + "/{asset_reference}"
	NodesPath             = PlanPath + "/nodes"
	NodePath              = NodesPath + "/{node_id}"
	WebhookDescriptorPath = PlanPath + "/webhook-descriptor"
	ContentPreviewPath    = PlanPath + "/content/preview"
	maximumBodyBytes      = 8 * 1024
)

var canonicalID = regexp.MustCompile(`^[1-9][0-9]{0,18}$`)

type methodRejectedKey struct{}

type Application interface {
	List(context.Context, int32, int32) (groupopsport.PlanPage, error)
	Detail(context.Context, int64) (groupopsport.Detail, error)
	Create(context.Context, groupopsport.CreatePlanCommand) (groupopsport.Detail, error)
	Update(context.Context, groupopsport.UpdatePlanCommand) (groupopsport.Detail, error)
	Activate(context.Context, groupopsport.TransitionCommand) (groupopsport.Detail, error)
	Pause(context.Context, groupopsport.TransitionCommand) (groupopsport.Detail, error)
	Archive(context.Context, groupopsport.TransitionCommand) (groupopsport.Detail, error)
	ListMembers(context.Context, int64, int32, int32) (groupopsport.MemberPage, error)
	AddMember(context.Context, groupopsport.MemberCommand) (groupopsport.Detail, error)
	RemoveMember(context.Context, groupopsport.MemberCommand) (groupopsport.Detail, error)
	ListGroupAssets(context.Context, int64, int32, int32) (groupopsport.GroupAssetPage, error)
	AddGroupAsset(context.Context, groupopsport.GroupAssetCommand) (groupopsport.Detail, error)
	RemoveGroupAsset(context.Context, groupopsport.GroupAssetCommand) (groupopsport.Detail, error)
	ListNodes(context.Context, int64, int32, int32) (groupopsport.NodePage, error)
	AddNode(context.Context, groupopsport.NodeCreateCommand) (groupopsport.Detail, error)
	UpdateNode(context.Context, groupopsport.NodeUpdateCommand) (groupopsport.Detail, error)
	RemoveNode(context.Context, groupopsport.NodeDeleteCommand) (groupopsport.Detail, error)
	GetWebhookDescriptor(context.Context, int64) (groupopsport.WebhookDescriptor, error)
	PutWebhookDescriptor(context.Context, groupopsport.WebhookDescriptorCommand) (groupopsport.Detail, error)
	Preview(context.Context, int64) (groupopsport.ContentValidation, error)
}

var _ Application = (*groupopsapp.Service)(nil)

type Handler struct {
	Application Application
	Runtime     RuntimeApplication
	Protocols   ProtocolAuthenticator
}

func New(application Application) *Handler { return &Handler{Application: application} }

func (h *Handler) ListPlans(w http.ResponseWriter, r *http.Request) {
	setHeaders(w)
	if !method(w, r, http.MethodGet) || !authorized(r, authport.CapabilityAdminRead) {
		authorization(w, r)
		return
	}
	limit, offset, ok := pageQuery(r)
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid_page")
		return
	}
	if !available(h) {
		unavailable(w)
		return
	}
	result, err := h.Application.List(r.Context(), limit, offset)
	if err != nil {
		applicationError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}
func (h *Handler) CreatePlan(w http.ResponseWriter, r *http.Request) {
	setHeaders(w)
	actor, ok := writeAuthorized(r)
	if !method(w, r, http.MethodPost) || !ok {
		authorization(w, r)
		return
	}
	key, ok := key(r)
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid_idempotency_key")
		return
	}
	var body struct {
		Name string `json:"name"`
	}
	if !decodeBody(r, &body) {
		writeError(w, http.StatusBadRequest, "invalid_request")
		return
	}
	if !available(h) {
		unavailable(w)
		return
	}
	result, err := h.Application.Create(r.Context(), groupopsport.CreatePlanCommand{Name: body.Name, Actor: actor, IdempotencyKey: key})
	if err != nil {
		applicationError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, result)
}
func (h *Handler) GetPlan(w http.ResponseWriter, r *http.Request) {
	setHeaders(w)
	if !method(w, r, http.MethodGet) || !authorized(r, authport.CapabilityAdminRead) {
		authorization(w, r)
		return
	}
	id, ok := planID(r, PlanPath)
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid_plan_id")
		return
	}
	if !available(h) {
		unavailable(w)
		return
	}
	result, err := h.Application.Detail(r.Context(), id)
	if err != nil {
		applicationError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}
func (h *Handler) UpdatePlan(w http.ResponseWriter, r *http.Request) {
	setHeaders(w)
	actor, ok := writeAuthorized(r)
	if !methodAny(w, r, http.MethodPatch, http.MethodPut) || !ok {
		authorization(w, r)
		return
	}
	id, ok := planID(r, PlanPath)
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid_plan_id")
		return
	}
	key, ok := key(r)
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid_idempotency_key")
		return
	}
	var body struct {
		ExpectedRevision int64  `json:"expected_revision"`
		Name             string `json:"name"`
	}
	if !decodeBody(r, &body) {
		writeError(w, http.StatusBadRequest, "invalid_request")
		return
	}
	if !available(h) {
		unavailable(w)
		return
	}
	result, err := h.Application.Update(r.Context(), groupopsport.UpdatePlanCommand{PlanID: id, ExpectedRevision: body.ExpectedRevision, Name: body.Name, Actor: actor, IdempotencyKey: key})
	if err != nil {
		applicationError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (h *Handler) Activate(w http.ResponseWriter, r *http.Request) {
	h.transition(w, r, PlanActivatePath, func(ctx context.Context, c groupopsport.TransitionCommand) (groupopsport.Detail, error) {
		return h.Application.Activate(ctx, c)
	})
}
func (h *Handler) Pause(w http.ResponseWriter, r *http.Request) {
	h.transition(w, r, PlanPausePath, func(ctx context.Context, c groupopsport.TransitionCommand) (groupopsport.Detail, error) {
		return h.Application.Pause(ctx, c)
	})
}
func (h *Handler) Archive(w http.ResponseWriter, r *http.Request) {
	h.transition(w, r, PlanArchivePath, func(ctx context.Context, c groupopsport.TransitionCommand) (groupopsport.Detail, error) {
		return h.Application.Archive(ctx, c)
	})
}
func (h *Handler) Enable(w http.ResponseWriter, r *http.Request) {
	h.transition(w, r, PlanEnablePath, func(ctx context.Context, c groupopsport.TransitionCommand) (groupopsport.Detail, error) {
		return h.Application.Activate(ctx, c)
	})
}
func (h *Handler) Disable(w http.ResponseWriter, r *http.Request) {
	h.transition(w, r, PlanDisablePath, func(ctx context.Context, c groupopsport.TransitionCommand) (groupopsport.Detail, error) {
		return h.Application.Pause(ctx, c)
	})
}
func (h *Handler) DeletePlan(w http.ResponseWriter, r *http.Request) {
	h.transitionWithMethod(w, r, PlanPath, http.MethodDelete, func(ctx context.Context, c groupopsport.TransitionCommand) (groupopsport.Detail, error) {
		return h.Application.Archive(ctx, c)
	})
}
func (h *Handler) transition(w http.ResponseWriter, r *http.Request, path string, call func(context.Context, groupopsport.TransitionCommand) (groupopsport.Detail, error)) {
	h.transitionWithMethod(w, r, path, http.MethodPost, call)
}
func (h *Handler) transitionWithMethod(w http.ResponseWriter, r *http.Request, path, requestMethod string, call func(context.Context, groupopsport.TransitionCommand) (groupopsport.Detail, error)) {
	setHeaders(w)
	actor, ok := writeAuthorized(r)
	if !method(w, r, requestMethod) || !ok {
		authorization(w, r)
		return
	}
	id, ok := planID(r, path)
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid_plan_id")
		return
	}
	key, ok := key(r)
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid_idempotency_key")
		return
	}
	var body struct {
		ExpectedRevision int64 `json:"expected_revision"`
	}
	if !decodeBody(r, &body) {
		writeError(w, http.StatusBadRequest, "invalid_request")
		return
	}
	if !available(h) {
		unavailable(w)
		return
	}
	result, err := call(r.Context(), groupopsport.TransitionCommand{PlanID: id, ExpectedRevision: body.ExpectedRevision, Actor: actor, IdempotencyKey: key})
	if err != nil {
		applicationError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (h *Handler) ListMembers(w http.ResponseWriter, r *http.Request) {
	setHeaders(w)
	if !method(w, r, http.MethodGet) || !authorized(r, authport.CapabilityAdminRead) {
		authorization(w, r)
		return
	}
	id, ok := planID(r, MembersPath)
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid_plan_id")
		return
	}
	limit, offset, ok := pageQuery(r)
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid_page")
		return
	}
	if !available(h) {
		unavailable(w)
		return
	}
	result, err := h.Application.ListMembers(r.Context(), id, limit, offset)
	if err != nil {
		applicationError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}
func (h *Handler) AddMember(w http.ResponseWriter, r *http.Request) {
	setHeaders(w)
	actor, ok := writeAuthorized(r)
	if !method(w, r, http.MethodPost) || !ok {
		authorization(w, r)
		return
	}
	id, ok := planID(r, MembersPath)
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid_plan_id")
		return
	}
	key, ok := key(r)
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid_idempotency_key")
		return
	}
	var body struct {
		ExpectedRevision int64 `json:"expected_revision"`
		StaffID          int64 `json:"staff_id"`
	}
	if !decodeBody(r, &body) {
		writeError(w, http.StatusBadRequest, "invalid_request")
		return
	}
	if !available(h) {
		unavailable(w)
		return
	}
	result, err := h.Application.AddMember(r.Context(), groupopsport.MemberCommand{PlanID: id, ExpectedRevision: body.ExpectedRevision, StaffID: body.StaffID, Actor: actor, IdempotencyKey: key})
	if err != nil {
		applicationError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}
func (h *Handler) RemoveMember(w http.ResponseWriter, r *http.Request) {
	setHeaders(w)
	actor, ok := writeAuthorized(r)
	if !method(w, r, http.MethodDelete) || !ok {
		authorization(w, r)
		return
	}
	id, ok := planID(r, MembersPath)
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid_plan_id")
		return
	}
	staff, ok := tailID(r.URL.Path, "/members/")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid_staff_id")
		return
	}
	key, ok := key(r)
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid_idempotency_key")
		return
	}
	var body struct {
		ExpectedRevision int64 `json:"expected_revision"`
	}
	if !decodeBody(r, &body) {
		writeError(w, http.StatusBadRequest, "invalid_request")
		return
	}
	if !available(h) {
		unavailable(w)
		return
	}
	result, err := h.Application.RemoveMember(r.Context(), groupopsport.MemberCommand{PlanID: id, ExpectedRevision: body.ExpectedRevision, StaffID: staff, Actor: actor, IdempotencyKey: key})
	if err != nil {
		applicationError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (h *Handler) ListGroupAssets(w http.ResponseWriter, r *http.Request) {
	h.listGroupAssets(w, r, GroupAssetsPath)
}
func (h *Handler) ListPlanGroups(w http.ResponseWriter, r *http.Request) {
	h.listGroupAssets(w, r, PlanGroupsPath)
}
func (h *Handler) listGroupAssets(w http.ResponseWriter, r *http.Request, path string) {
	setHeaders(w)
	if !method(w, r, http.MethodGet) || !authorized(r, authport.CapabilityAdminRead) {
		authorization(w, r)
		return
	}
	id, ok := planID(r, path)
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid_plan_id")
		return
	}
	limit, offset, ok := pageQuery(r)
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid_page")
		return
	}
	if !available(h) {
		unavailable(w)
		return
	}
	result, err := h.Application.ListGroupAssets(r.Context(), id, limit, offset)
	if err != nil {
		applicationError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}
func (h *Handler) AddGroupAsset(w http.ResponseWriter, r *http.Request) {
	h.addGroupAsset(w, r, GroupAssetsPath)
}
func (h *Handler) AddPlanGroup(w http.ResponseWriter, r *http.Request) {
	h.addGroupAsset(w, r, PlanGroupsPath)
}
func (h *Handler) addGroupAsset(w http.ResponseWriter, r *http.Request, path string) {
	setHeaders(w)
	actor, ok := writeAuthorized(r)
	if !method(w, r, http.MethodPost) || !ok {
		authorization(w, r)
		return
	}
	id, ok := planID(r, path)
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid_plan_id")
		return
	}
	key, ok := key(r)
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid_idempotency_key")
		return
	}
	var body struct {
		ExpectedRevision int64  `json:"expected_revision"`
		AssetReference   string `json:"asset_reference"`
	}
	if !decodeBody(r, &body) {
		writeError(w, http.StatusBadRequest, "invalid_request")
		return
	}
	if !available(h) {
		unavailable(w)
		return
	}
	result, err := h.Application.AddGroupAsset(r.Context(), groupopsport.GroupAssetCommand{PlanID: id, ExpectedRevision: body.ExpectedRevision, AssetRef: body.AssetReference, Actor: actor, IdempotencyKey: key})
	if err != nil {
		applicationError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}
func (h *Handler) RemoveGroupAsset(w http.ResponseWriter, r *http.Request) {
	h.removeGroupAsset(w, r, GroupAssetsPath, "/group-assets/")
}
func (h *Handler) RemovePlanGroup(w http.ResponseWriter, r *http.Request) {
	h.removeGroupAsset(w, r, PlanGroupsPath, "/groups/")
}
func (h *Handler) removeGroupAsset(w http.ResponseWriter, r *http.Request, path, marker string) {
	setHeaders(w)
	actor, ok := writeAuthorized(r)
	if !method(w, r, http.MethodDelete) || !ok {
		authorization(w, r)
		return
	}
	id, ok := planID(r, path)
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid_plan_id")
		return
	}
	ref, ok := tailOpaque(r.URL.Path, marker)
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid_asset_reference")
		return
	}
	key, ok := key(r)
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid_idempotency_key")
		return
	}
	var body struct {
		ExpectedRevision int64 `json:"expected_revision"`
	}
	if !decodeBody(r, &body) {
		writeError(w, http.StatusBadRequest, "invalid_request")
		return
	}
	if !available(h) {
		unavailable(w)
		return
	}
	result, err := h.Application.RemoveGroupAsset(r.Context(), groupopsport.GroupAssetCommand{PlanID: id, ExpectedRevision: body.ExpectedRevision, AssetRef: ref, Actor: actor, IdempotencyKey: key})
	if err != nil {
		applicationError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (h *Handler) ListNodes(w http.ResponseWriter, r *http.Request) {
	setHeaders(w)
	if !method(w, r, http.MethodGet) || !authorized(r, authport.CapabilityAdminRead) {
		authorization(w, r)
		return
	}
	id, ok := planID(r, NodesPath)
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid_plan_id")
		return
	}
	limit, offset, ok := pageQuery(r)
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid_page")
		return
	}
	if !available(h) {
		unavailable(w)
		return
	}
	result, err := h.Application.ListNodes(r.Context(), id, limit, offset)
	if err != nil {
		applicationError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}
func (h *Handler) AddNode(w http.ResponseWriter, r *http.Request) {
	setHeaders(w)
	actor, ok := writeAuthorized(r)
	if !method(w, r, http.MethodPost) || !ok {
		authorization(w, r)
		return
	}
	id, ok := planID(r, NodesPath)
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid_plan_id")
		return
	}
	key, ok := key(r)
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid_idempotency_key")
		return
	}
	command, ok := nodeCreateBody(r, id, actor, key)
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid_request")
		return
	}
	if !available(h) {
		unavailable(w)
		return
	}
	result, err := h.Application.AddNode(r.Context(), command)
	if err != nil {
		applicationError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}
func (h *Handler) UpdateNode(w http.ResponseWriter, r *http.Request) {
	setHeaders(w)
	actor, ok := writeAuthorized(r)
	if !methodAny(w, r, http.MethodPatch, http.MethodPut) || !ok {
		authorization(w, r)
		return
	}
	id, ok := planID(r, NodesPath)
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid_plan_id")
		return
	}
	node, ok := tailID(r.URL.Path, "/nodes/")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid_node_id")
		return
	}
	key, ok := key(r)
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid_idempotency_key")
		return
	}
	created, ok := nodeCreateBody(r, id, actor, key)
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid_request")
		return
	}
	if !available(h) {
		unavailable(w)
		return
	}
	result, err := h.Application.UpdateNode(r.Context(), groupopsport.NodeUpdateCommand{PlanID: id, NodeID: node, ExpectedRevision: created.ExpectedRevision, Position: created.Position, Kind: created.Kind, MessageText: created.MessageText, DelayMinutes: created.DelayMinutes, MaterialRef: created.MaterialRef, Actor: actor, IdempotencyKey: key})
	if err != nil {
		applicationError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}
func (h *Handler) RemoveNode(w http.ResponseWriter, r *http.Request) {
	setHeaders(w)
	actor, ok := writeAuthorized(r)
	if !method(w, r, http.MethodDelete) || !ok {
		authorization(w, r)
		return
	}
	id, ok := planID(r, NodesPath)
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid_plan_id")
		return
	}
	node, ok := tailID(r.URL.Path, "/nodes/")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid_node_id")
		return
	}
	key, ok := key(r)
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid_idempotency_key")
		return
	}
	var body struct {
		ExpectedRevision int64 `json:"expected_revision"`
	}
	if !decodeBody(r, &body) {
		writeError(w, http.StatusBadRequest, "invalid_request")
		return
	}
	if !available(h) {
		unavailable(w)
		return
	}
	result, err := h.Application.RemoveNode(r.Context(), groupopsport.NodeDeleteCommand{PlanID: id, NodeID: node, ExpectedRevision: body.ExpectedRevision, Actor: actor, IdempotencyKey: key})
	if err != nil {
		applicationError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (h *Handler) GetWebhookDescriptor(w http.ResponseWriter, r *http.Request) {
	h.getWebhookDescriptor(w, r, WebhookDescriptorPath)
}
func (h *Handler) GetWebhook(w http.ResponseWriter, r *http.Request) {
	h.getWebhookDescriptor(w, r, PlanWebhookPath)
}
func (h *Handler) getWebhookDescriptor(w http.ResponseWriter, r *http.Request, path string) {
	setHeaders(w)
	if !method(w, r, http.MethodGet) || !authorized(r, authport.CapabilityAdminRead) {
		authorization(w, r)
		return
	}
	id, ok := planID(r, path)
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid_plan_id")
		return
	}
	if !available(h) {
		unavailable(w)
		return
	}
	result, err := h.Application.GetWebhookDescriptor(r.Context(), id)
	if err != nil {
		applicationError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, struct {
		groupopsport.WebhookDescriptor
		groupopsport.Safety
	}{result, groupopsport.LocalSafety()})
}
func (h *Handler) PutWebhookDescriptor(w http.ResponseWriter, r *http.Request) {
	setHeaders(w)
	actor, ok := writeAuthorized(r)
	if !method(w, r, http.MethodPut) || !ok {
		authorization(w, r)
		return
	}
	id, ok := planID(r, WebhookDescriptorPath)
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid_plan_id")
		return
	}
	key, ok := key(r)
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid_idempotency_key")
		return
	}
	var body struct {
		ExpectedRevision int64  `json:"expected_revision"`
		Reference        string `json:"reference"`
	}
	if !decodeBody(r, &body) {
		writeError(w, http.StatusBadRequest, "invalid_request")
		return
	}
	if !available(h) {
		unavailable(w)
		return
	}
	result, err := h.Application.PutWebhookDescriptor(r.Context(), groupopsport.WebhookDescriptorCommand{PlanID: id, ExpectedRevision: body.ExpectedRevision, Reference: body.Reference, Actor: actor, IdempotencyKey: key})
	if err != nil {
		applicationError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}
func (h *Handler) Preview(w http.ResponseWriter, r *http.Request) {
	setHeaders(w)
	if !method(w, r, http.MethodPost) || !authorized(r, authport.CapabilityAdminRead) {
		authorization(w, r)
		return
	}
	id, ok := planID(r, ContentPreviewPath)
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid_plan_id")
		return
	}
	if !emptyBody(r) {
		writeError(w, http.StatusBadRequest, "invalid_request")
		return
	}
	if !available(h) {
		unavailable(w)
		return
	}
	result, err := h.Application.Preview(r.Context(), id)
	if err != nil {
		applicationError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func nodeCreateBody(r *http.Request, id, actor int64, key string) (groupopsport.NodeCreateCommand, bool) {
	var body struct {
		ExpectedRevision int64                 `json:"expected_revision"`
		Position         int32                 `json:"position"`
		Kind             groupopsport.NodeKind `json:"kind"`
		MessageText      string                `json:"message_text"`
		DelayMinutes     int32                 `json:"delay_minutes"`
		MaterialRef      string                `json:"material_reference"`
	}
	if !decodeBody(r, &body) {
		return groupopsport.NodeCreateCommand{}, false
	}
	return groupopsport.NodeCreateCommand{PlanID: id, ExpectedRevision: body.ExpectedRevision, Position: body.Position, Kind: body.Kind, MessageText: body.MessageText, DelayMinutes: body.DelayMinutes, MaterialRef: body.MaterialRef, Actor: actor, IdempotencyKey: key}, true
}

func method(w http.ResponseWriter, r *http.Request, want string) bool {
	if r != nil && r.Method == want {
		return true
	}
	if r != nil {
		*r = *r.WithContext(context.WithValue(r.Context(), methodRejectedKey{}, true))
	}
	w.Header().Set("Allow", want)
	writeError(w, http.StatusMethodNotAllowed, "method_not_allowed")
	return false
}
func methodAny(w http.ResponseWriter, r *http.Request, allowed ...string) bool {
	if r != nil {
		for _, candidate := range allowed {
			if r.Method == candidate {
				return true
			}
		}
		*r = *r.WithContext(context.WithValue(r.Context(), methodRejectedKey{}, true))
	}
	w.Header().Set("Allow", strings.Join(allowed, ", "))
	writeError(w, http.StatusMethodNotAllowed, "method_not_allowed")
	return false
}
func authorized(r *http.Request, capability authport.Capability) bool {
	if r == nil {
		return false
	}
	p, pok := authport.PrincipalFromContext(r.Context())
	a, aok := authport.AuthorizationFromContext(r.Context())
	return pok && p.AdminUserID > 0 && (p.Role == authport.RoleAdmin || p.Role == authport.RoleOps) && aok && a.Capability == capability && a.Scope == authport.ScopeGlobal && a.OwnerStaffID == 0
}
func writeAuthorized(r *http.Request) (int64, bool) {
	if !authorized(r, authport.CapabilityOperationsManage) {
		return 0, false
	}
	p, _ := authport.PrincipalFromContext(r.Context())
	return p.AdminUserID, true
}
func authorization(w http.ResponseWriter, r *http.Request) {
	if r != nil && r.Context().Value(methodRejectedKey{}) == true {
		return
	}
	if r == nil {
		writeError(w, http.StatusUnauthorized, "authentication_required")
		return
	}
	if _, ok := authport.PrincipalFromContext(r.Context()); !ok {
		writeError(w, http.StatusUnauthorized, "authentication_required")
		return
	}
	writeError(w, http.StatusForbidden, "permission_denied")
}
func planID(r *http.Request, pattern string) (int64, bool) {
	if r == nil {
		return 0, false
	}
	prefix := strings.Split(pattern, "{plan_id}")[0]
	suffix := strings.Split(pattern, "{plan_id}")[1]
	path := r.URL.Path
	if !strings.HasPrefix(path, prefix) || !strings.HasSuffix(path, suffix) {
		return 0, false
	}
	value := strings.TrimSuffix(strings.TrimPrefix(path, prefix), suffix)
	if strings.ContainsAny(value, "/\\\x00\r\n") || !canonicalID.MatchString(value) {
		return 0, false
	}
	parsed, err := strconv.ParseInt(value, 10, 64)
	return parsed, err == nil && parsed > 0
}
func tailID(path, marker string) (int64, bool) {
	value := strings.TrimPrefix(path[:], strings.Split(path, marker)[0]+marker)
	if !canonicalID.MatchString(value) {
		return 0, false
	}
	parsed, err := strconv.ParseInt(value, 10, 64)
	return parsed, err == nil && parsed > 0
}
func tailOpaque(path, marker string) (string, bool) {
	parts := strings.Split(path, marker)
	if len(parts) != 2 || parts[1] == "" || strings.ContainsAny(parts[1], "/\\\x00\r\n") {
		return "", false
	}
	return parts[1], true
}
func pageQuery(r *http.Request) (int32, int32, bool) {
	if r == nil {
		return 0, 0, false
	}
	values := r.URL.Query()
	limit, offset := int32(groupopsapp.DefaultLimit), int32(0)
	for name, target := range map[string]*int32{"limit": &limit, "offset": &offset} {
		raw, ok := values[name]
		if !ok {
			continue
		}
		if len(raw) != 1 || raw[0] == "" {
			return 0, 0, false
		}
		parsed, err := strconv.ParseInt(raw[0], 10, 32)
		if err != nil {
			return 0, 0, false
		}
		*target = int32(parsed)
	}
	for name := range values {
		if name != "limit" && name != "offset" {
			return 0, 0, false
		}
	}
	return limit, offset, limit >= 1 && limit <= groupopsapp.MaximumLimit && offset >= 0 && offset <= groupopsapp.MaximumOffset
}
func key(r *http.Request) (string, bool) {
	if r == nil {
		return "", false
	}
	value := r.Header.Values("Idempotency-Key")
	if len(value) != 1 || !utf8.ValidString(value[0]) || strings.TrimSpace(value[0]) != value[0] || utf8.RuneCountInString(value[0]) < 16 || utf8.RuneCountInString(value[0]) > 128 {
		return "", false
	}
	return value[0], true
}
func decodeBody(r *http.Request, to any) bool {
	if r == nil || to == nil || r.Body == nil {
		return false
	}
	contentType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || contentType != "application/json" {
		return false
	}
	body := http.MaxBytesReader(nil, r.Body, maximumBodyBytes)
	decoder := json.NewDecoder(body)
	decoder.DisallowUnknownFields()
	if decoder.Decode(to) != nil {
		return false
	}
	var extra any
	return errors.Is(decoder.Decode(&extra), io.EOF)
}
func emptyBody(r *http.Request) bool {
	if r == nil || r.Body == nil {
		return true
	}
	body := http.MaxBytesReader(nil, r.Body, maximumBodyBytes)
	raw, err := io.ReadAll(body)
	return err == nil && len(bytes.TrimSpace(raw)) == 0
}
func available(h *Handler) bool { return h != nil && h.Application != nil }
func setHeaders(w http.ResponseWriter) {
	w.Header().Set("Cache-Control", "private, no-store")
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("X-Content-Type-Options", "nosniff")
}
func writeJSON(w http.ResponseWriter, status int, value any) {
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
func writeError(w http.ResponseWriter, status int, code string) {
	writeJSON(w, status, map[string]any{"ok": false, "error": map[string]string{"code": code}, "provider_execution_eligible": false, "real_external_call_executed": false})
}
func unavailable(w http.ResponseWriter) {
	writeError(w, http.StatusServiceUnavailable, "group_ops_unavailable")
}
func applicationError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, groupopsapp.ErrInvalid):
		writeError(w, http.StatusBadRequest, "invalid_request")
	case errors.Is(err, groupopsapp.ErrNotFound):
		writeError(w, http.StatusNotFound, "plan_not_found")
	case errors.Is(err, groupopsapp.ErrConflict), errors.Is(err, groupopsapp.ErrStateConflict):
		writeError(w, http.StatusConflict, "operations_conflict")
	default:
		unavailable(w)
	}
}
