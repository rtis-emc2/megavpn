package domain

import "time"

type PlatformUser struct {
	ID          string     `json:"id"`
	Username    string     `json:"username"`
	Email       string     `json:"email"`
	DisplayName string     `json:"display_name"`
	Status      string     `json:"status"`
	AuthSource  string     `json:"auth_source"`
	LastLoginAt *time.Time `json:"last_login_at,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

type PlatformUserAuth struct {
	User         PlatformUser
	PasswordHash string
}

type PlatformUserRecord struct {
	PlatformUser
	RoleCodes []string `json:"roles"`
}

type UserSession struct {
	ID        string     `json:"id"`
	UserID    string     `json:"user_id"`
	ExpiresAt time.Time  `json:"expires_at"`
	RevokedAt *time.Time `json:"revoked_at,omitempty"`
	CreatedAt time.Time  `json:"created_at"`
}

type UserSessionRecord struct {
	UserSession
	Username    string `json:"username"`
	Email       string `json:"email"`
	DisplayName string `json:"display_name"`
	IP          string `json:"ip"`
	UserAgent   string `json:"user_agent"`
}

type AuthContext struct {
	User            PlatformUser `json:"user"`
	Session         UserSession  `json:"session"`
	RoleCodes       []string     `json:"roles"`
	PermissionCodes []string     `json:"permissions"`
}
