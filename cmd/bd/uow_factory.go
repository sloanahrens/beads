package main

import "time"

// providerCloseTimeout bounds the close of a provider a caller built itself. It
// is not a drain budget: whatever was running on the provider has already
// drained by the time the close is issued.
const providerCloseTimeout = 10 * time.Second
