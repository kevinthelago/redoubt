package schedule

import "testing"

func TestIsVaultHostFalseByDefault(t *testing.T) {
	t.Setenv("REDOUBT_IS_VAULT", "")
	if IsVaultHost() {
		t.Fatal("IsVaultHost() must return false when REDOUBT_IS_VAULT is unset")
	}
}

func TestIsVaultHostRequiresExactValue(t *testing.T) {
	for _, v := range []string{"true", "yes", "0", "vault", "TRUE", "1 "} {
		t.Setenv("REDOUBT_IS_VAULT", v)
		if IsVaultHost() {
			t.Errorf("IsVaultHost() must return false for REDOUBT_IS_VAULT=%q", v)
		}
	}
}

func TestIsVaultHostTrueWhenSet(t *testing.T) {
	t.Setenv("REDOUBT_IS_VAULT", "1")
	if !IsVaultHost() {
		t.Fatal("IsVaultHost() must return true when REDOUBT_IS_VAULT=1")
	}
}
