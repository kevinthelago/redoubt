package keystore_test

// TestMain is omitted here intentionally.
// Tests use the real OS keyring; clearKeyring() cleans up before and after each test.
// For CI environments where keyring access is unavailable (headless Linux), the
// REDOUBT_TEST_SKIP_KEYRING env var can be set to skip keyring tests.
//
// The go-keyring package automatically falls back to an in-memory mock on
// systems where libsecret is unavailable, so most CI should work without
// additional configuration.
