package main

import (
	"net/http"

	api "github.com/qianlan33333-png/AI-CRM-v2/internal/api/candidate/generated"
)

func (handler *candidateHandler) GetCustomerContactPolicy(writer http.ResponseWriter, request *http.Request, customerID api.CustomerID) {
	handler.contactPolicy.Get(writer, request, customerID)
}

func (handler *candidateHandler) PutCustomerContactPolicy(writer http.ResponseWriter, request *http.Request, customerID api.CustomerID, params api.PutCustomerContactPolicyParams) {
	handler.contactPolicy.Set(writer, request, customerID, string(params.IdempotencyKey))
}

func (handler *candidateHandler) DeleteCustomerContactPolicy(writer http.ResponseWriter, request *http.Request, customerID api.CustomerID, params api.DeleteCustomerContactPolicyParams) {
	handler.contactPolicy.Clear(writer, request, customerID, string(params.IdempotencyKey))
}
