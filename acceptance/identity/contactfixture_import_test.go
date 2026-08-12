package identity_test

import (
	"testing"

	"github.com/qianlan33333-png/AI-CRM-v2/acceptance/contactfixture"
)

func TestContactFixtureIsImportableByIdentityAcceptance(t *testing.T) {
	_ = contactfixture.CreateCustomer
	t.Log("Contact-owned fixture import compiled")
}
