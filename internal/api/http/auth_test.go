package http

import (
	"testing"

	"github.com/rtis-emc2/megavpn/internal/domain"
)

func TestAuthContextHasPermission(t *testing.T) {
	cases := []struct {
		name       string
		roles      []string
		permission []string
		required   string
		want       bool
	}{
		{name: "empty requirement passes", roles: []string{"readonly"}, required: "", want: true},
		{name: "explicit permission passes", roles: []string{"engineer"}, permission: []string{"firewall.read"}, required: "firewall.read", want: true},
		{name: "missing permission fails", roles: []string{"engineer"}, permission: []string{"firewall.read"}, required: "firewall.apply", want: false},
		{name: "superadmin is full access fallback", roles: []string{"superadmin"}, required: "firewall.apply", want: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			authCtx := domain.AuthContext{RoleCodes: tc.roles, PermissionCodes: tc.permission}
			if got := authContextHasPermission(authCtx, tc.required); got != tc.want {
				t.Fatalf("authContextHasPermission() = %v, want %v", got, tc.want)
			}
		})
	}
}
