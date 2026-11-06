//go:build cgo

package main

import "testing"

// Generic update close policy belongs to shared lifecycle conformance. This
// file used to pin the batch update grammar's force token; that grammar went
// with the bd batch verb (be-xu2.6.6). The override survives as a flag on the
// kept bd update verb, so pin that it is still registered. Its direct and
// proxied behaviour lives in update_close_policy_integration_test.go and
// update_proxied_server_test.go.
func TestUpdateClosePolicyForceFlagRegistered(t *testing.T) {
	if updateCmd.Flags().Lookup("force") == nil {
		t.Fatal("bd update lost its close-policy --force flag")
	}
}
