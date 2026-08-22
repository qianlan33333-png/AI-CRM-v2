package main

import "net/http"

// groupOpsHTTP keeps the local-only Group Ops transport at the composition
// boundary while the legacy router supplies authentication and CSRF middleware.
type groupOpsHTTP interface {
	ListPlans(http.ResponseWriter, *http.Request)
	CreatePlan(http.ResponseWriter, *http.Request)
	GetPlan(http.ResponseWriter, *http.Request)
	UpdatePlan(http.ResponseWriter, *http.Request)
	Activate(http.ResponseWriter, *http.Request)
	Pause(http.ResponseWriter, *http.Request)
	Archive(http.ResponseWriter, *http.Request)
	ListMembers(http.ResponseWriter, *http.Request)
	AddMember(http.ResponseWriter, *http.Request)
	RemoveMember(http.ResponseWriter, *http.Request)
	ListGroupAssets(http.ResponseWriter, *http.Request)
	AddGroupAsset(http.ResponseWriter, *http.Request)
	RemoveGroupAsset(http.ResponseWriter, *http.Request)
	ListNodes(http.ResponseWriter, *http.Request)
	AddNode(http.ResponseWriter, *http.Request)
	UpdateNode(http.ResponseWriter, *http.Request)
	RemoveNode(http.ResponseWriter, *http.Request)
	GetWebhookDescriptor(http.ResponseWriter, *http.Request)
	PutWebhookDescriptor(http.ResponseWriter, *http.Request)
	Preview(http.ResponseWriter, *http.Request)
}
