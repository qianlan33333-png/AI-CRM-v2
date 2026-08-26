// Package groupopsprovider holds the protocol-neutral Group Ops Provider
// boundary and the approved WeCom group-message protocol adapter.
package groupopsprovider

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"

	eer "github.com/qianlan33333-png/AI-CRM-v2/internal/externaleffects/port"
	groupopsport "github.com/qianlan33333-png/AI-CRM-v2/internal/groupops/port"
)

var ErrInvalidDispatch = errors.New("group ops invalid dispatch")

// DispatchAdapter maps the narrow Group Ops Provider result to the EER
// terminal vocabulary. It never creates a delivery_proven claim.
type DispatchAdapter struct {
	provider groupopsport.DispatchProvider
	request  groupopsport.DispatchRequest
}

func NewDispatchAdapter(provider groupopsport.DispatchProvider, execution groupopsport.DispatchExecution) (*DispatchAdapter, error) {
	if provider == nil || !validExecution(execution) {
		return nil, ErrInvalidDispatch
	}
	return &DispatchAdapter{provider: provider, request: groupopsport.DispatchRequest{
		ExecutionID: execution.ExecutionID, ExternalEffectID: execution.ExternalEffectID,
		TargetReference: execution.TargetReference, SenderUserID: execution.SenderUserID, ContentSnapshot: append(json.RawMessage(nil), execution.ContentSnapshot...),
		ContentDigest: execution.ContentDigest, MaterialSnapshot: append(json.RawMessage(nil), execution.MaterialSnapshot...),
		MaterialDigest: execution.MaterialDigest,
	}}, nil
}

func (adapter *DispatchAdapter) Execute(ctx context.Context, envelope eer.EffectEnvelope, _ eer.Attempt) (eer.AdapterResult, error) {
	effectID := ""
	if adapter != nil {
		effectID = adapter.request.ExternalEffectID
	}
	if adapter == nil || adapter.provider == nil || ctx == nil || !validRequest(adapter.request) || envelope.Fingerprint() == "" {
		return eer.AdapterResult{Completion: eer.CompletionFinalFailed, ReceiptDigest: localReceipt("pre-dispatch", effectID)}, nil
	}
	result, err := adapter.provider.Dispatch(ctx, copyRequest(adapter.request))
	if err != nil {
		return eer.AdapterResult{BusinessCallDispatched: result.BusinessCallDispatched, RealExternalCallExecuted: result.RealExternalCallExecuted && result.BusinessCallDispatched}, err
	}
	mapped, err := adapterResult(result, adapter.request.ExternalEffectID)
	if err != nil {
		// An unclassifiable result cannot create call evidence by inference.
		return eer.AdapterResult{BusinessCallDispatched: result.BusinessCallDispatched, RealExternalCallExecuted: result.RealExternalCallExecuted && result.BusinessCallDispatched}, err
	}
	return mapped, nil
}

func adapterResult(result groupopsport.DispatchProviderResult, effectID string) (eer.AdapterResult, error) {
	receipt := eer.Digest(result.ReceiptDigest)
	if result.RealExternalCallExecuted && !result.BusinessCallDispatched {
		return eer.AdapterResult{}, ErrInvalidDispatch
	}
	switch result.Outcome {
	case groupopsport.DispatchPreDispatchFailure:
		if result.BusinessCallDispatched || result.RealExternalCallExecuted {
			return eer.AdapterResult{}, ErrInvalidDispatch
		}
		if receipt == "" {
			receipt = localReceipt("pre-dispatch", effectID)
		}
		if !validDigest(receipt) {
			return eer.AdapterResult{}, ErrInvalidDispatch
		}
		return eer.AdapterResult{Completion: eer.CompletionFinalFailed, ReceiptDigest: receipt}, nil
	case groupopsport.DispatchProviderAccepted:
		if !validDigest(receipt) || !result.BusinessCallDispatched || !result.RealExternalCallExecuted {
			return eer.AdapterResult{}, ErrInvalidDispatch
		}
		return eer.AdapterResult{Completion: eer.CompletionExecuted, ReceiptDigest: receipt, ResultReferenceDigest: receipt, BusinessCallDispatched: result.BusinessCallDispatched, RealExternalCallExecuted: result.RealExternalCallExecuted}, nil
	case groupopsport.DispatchOutcomeUnknown:
		if !result.BusinessCallDispatched || !result.RealExternalCallExecuted {
			return eer.AdapterResult{}, ErrInvalidDispatch
		}
		if receipt == "" {
			receipt = localReceipt("outcome-unknown", effectID)
		}
		if !validDigest(receipt) {
			return eer.AdapterResult{}, ErrInvalidDispatch
		}
		return eer.AdapterResult{Completion: eer.CompletionOutcomeUnknown, ReceiptDigest: receipt, BusinessCallDispatched: result.BusinessCallDispatched, RealExternalCallExecuted: result.RealExternalCallExecuted}, nil
	case groupopsport.DispatchProviderRejected:
		if !validDigest(receipt) || !result.BusinessCallDispatched || !result.RealExternalCallExecuted {
			return eer.AdapterResult{}, ErrInvalidDispatch
		}
		return eer.AdapterResult{Completion: eer.CompletionFinalFailed, ReceiptDigest: receipt, ResultReferenceDigest: receipt, BusinessCallDispatched: result.BusinessCallDispatched, RealExternalCallExecuted: result.RealExternalCallExecuted}, nil
	default:
		return eer.AdapterResult{}, ErrInvalidDispatch
	}
}

func (adapter *DispatchAdapter) Request() groupopsport.DispatchRequest {
	if adapter == nil {
		return groupopsport.DispatchRequest{}
	}
	return copyRequest(adapter.request)
}

func validExecution(value groupopsport.DispatchExecution) bool {
	return value.ExecutionID > 0 && value.State == groupopsport.ExecutionAccepted && validRequest(groupopsport.DispatchRequest{
		ExecutionID: value.ExecutionID, ExternalEffectID: value.ExternalEffectID, TargetReference: value.TargetReference, SenderUserID: value.SenderUserID,
		ContentSnapshot: value.ContentSnapshot, ContentDigest: value.ContentDigest, MaterialSnapshot: value.MaterialSnapshot, MaterialDigest: value.MaterialDigest,
	})
}

func validRequest(value groupopsport.DispatchRequest) bool {
	return value.ExecutionID > 0 && strings.TrimSpace(value.ExternalEffectID) != "" && strings.TrimSpace(value.TargetReference) != "" && strings.TrimSpace(value.SenderUserID) == value.SenderUserID && value.SenderUserID != "" &&
		jsonObject(value.ContentSnapshot) && jsonObject(value.MaterialSnapshot) && validDigest(eer.Digest(value.ContentDigest)) && validDigest(eer.Digest(value.MaterialDigest))
}

func jsonObject(value json.RawMessage) bool {
	var decoded map[string]any
	return json.Unmarshal(value, &decoded) == nil && decoded != nil
}

func validDigest(value eer.Digest) bool {
	if !strings.HasPrefix(string(value), "sha256:") || len(value) != len("sha256:")+sha256.Size*2 {
		return false
	}
	_, err := hex.DecodeString(strings.TrimPrefix(string(value), "sha256:"))
	return err == nil
}

func localReceipt(label, effectID string) eer.Digest {
	sum := sha256.Sum256([]byte("group-ops-dispatch-v1\x00" + label + "\x00" + effectID))
	return eer.Digest("sha256:" + hex.EncodeToString(sum[:]))
}

func copyRequest(value groupopsport.DispatchRequest) groupopsport.DispatchRequest {
	value.ContentSnapshot = append(json.RawMessage(nil), value.ContentSnapshot...)
	value.MaterialSnapshot = append(json.RawMessage(nil), value.MaterialSnapshot...)
	return value
}
