package postgres

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/rtis-emc2/megavpn/internal/domain"
	"github.com/rtis-emc2/megavpn/internal/platform/id"
)

func (s *Store) EnsureBootstrapPlatformUser(ctx context.Context, username, email, displayName, passwordHash string) (domain.PlatformUser, bool, error) {
	username = normalizeUsername(username)
	email = normalizeEmail(email)
	if username == "" || strings.TrimSpace(passwordHash) == "" {
		return domain.PlatformUser{}, false, errors.New("username and password hash are required")
	}
	if email == "" {
		email = username + "@rtis.local"
	}
	displayName = strings.TrimSpace(displayName)
	if displayName == "" {
		displayName = "Superadmin"
	}

	tx, err := s.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return domain.PlatformUser{}, false, err
	}
	defer tx.Rollback(ctx)

	now := time.Now().UTC()
	user := domain.PlatformUser{}
	created := false
	var userCount int
	if err := tx.QueryRow(ctx, `select count(*) from platform_users`).Scan(&userCount); err != nil {
		return domain.PlatformUser{}, false, err
	}
	err = tx.QueryRow(ctx, `select id,username,email,display_name,status,auth_source,last_login_at,created_at,updated_at from platform_users where username=$1 or email=$2`, username, email).
		Scan(&user.ID, &user.Username, &user.Email, &user.DisplayName, &user.Status, &user.AuthSource, &user.LastLoginAt, &user.CreatedAt, &user.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		if userCount > 0 {
			return domain.PlatformUser{}, false, errors.New("bootstrap admin can only be created when no platform users exist")
		}
		created = true
		user = domain.PlatformUser{
			ID:          id.New(),
			Username:    username,
			Email:       email,
			DisplayName: displayName,
			Status:      "active",
			AuthSource:  "local",
			CreatedAt:   now,
			UpdatedAt:   now,
		}
		_, err = tx.Exec(ctx, `insert into platform_users(id,username,email,display_name,status,password_hash,auth_source,created_at,updated_at) values($1,$2,$3,$4,$5,$6,$7,$8,$9)`,
			user.ID, user.Username, user.Email, user.DisplayName, user.Status, passwordHash, user.AuthSource, user.CreatedAt, user.UpdatedAt)
		if err != nil {
			return domain.PlatformUser{}, false, err
		}
	} else if err != nil {
		return domain.PlatformUser{}, false, err
	} else {
		return user, false, tx.Commit(ctx)
	}

	var roleID string
	if err := tx.QueryRow(ctx, `select id from roles where code='superadmin'`).Scan(&roleID); err != nil {
		return domain.PlatformUser{}, false, err
	}
	if _, err := tx.Exec(ctx, `insert into platform_user_roles(user_id,role_id,assigned_by,created_at) values($1,$2,null,now()) on conflict(user_id, role_id) do nothing`, user.ID, roleID); err != nil {
		return domain.PlatformUser{}, false, err
	}

	if err := tx.Commit(ctx); err != nil {
		return domain.PlatformUser{}, false, err
	}
	if created {
		_, _ = s.CreateAuditForUser(ctx, &user.ID, "auth.bootstrap_admin.create", "platform_user", &user.ID, "bootstrap admin created")
	}
	return user, created, nil
}

func (s *Store) GetPlatformUserForAuth(ctx context.Context, login string) (domain.PlatformUserAuth, error) {
	login = strings.TrimSpace(login)
	username := normalizeUsername(login)
	email := normalizeEmail(login)
	var x domain.PlatformUserAuth
	err := s.db.QueryRow(ctx, `select id,username,email,display_name,status,auth_source,last_login_at,created_at,updated_at,password_hash from platform_users where username=$1 or email=$2`, username, email).
		Scan(&x.User.ID, &x.User.Username, &x.User.Email, &x.User.DisplayName, &x.User.Status, &x.User.AuthSource, &x.User.LastLoginAt, &x.User.CreatedAt, &x.User.UpdatedAt, &x.PasswordHash)
	return x, err
}

func (s *Store) GetPlatformUserByIDForAuth(ctx context.Context, userID string) (domain.PlatformUserAuth, error) {
	var x domain.PlatformUserAuth
	err := s.db.QueryRow(ctx, `select id,username,email,display_name,status,auth_source,last_login_at,created_at,updated_at,password_hash from platform_users where id=$1`, strings.TrimSpace(userID)).
		Scan(&x.User.ID, &x.User.Username, &x.User.Email, &x.User.DisplayName, &x.User.Status, &x.User.AuthSource, &x.User.LastLoginAt, &x.User.CreatedAt, &x.User.UpdatedAt, &x.PasswordHash)
	return x, err
}

func (s *Store) ListPlatformUsers(ctx context.Context, limit int) ([]domain.PlatformUserRecord, error) {
	if limit <= 0 || limit > 200 {
		limit = 100
	}
	rows, err := s.db.Query(ctx, `select id,username,email,display_name,status,auth_source,last_login_at,created_at,updated_at from platform_users order by created_at asc limit $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.PlatformUserRecord{}
	for rows.Next() {
		var x domain.PlatformUserRecord
		if err := rows.Scan(&x.ID, &x.Username, &x.Email, &x.DisplayName, &x.Status, &x.AuthSource, &x.LastLoginAt, &x.CreatedAt, &x.UpdatedAt); err != nil {
			return nil, err
		}
		roles, err := s.listPlatformUserRoles(ctx, x.ID)
		if err != nil {
			return nil, err
		}
		x.RoleCodes = roles
		out = append(out, x)
	}
	return out, rows.Err()
}

func (s *Store) GetPlatformUserRecord(ctx context.Context, userID string) (domain.PlatformUserRecord, error) {
	return s.getPlatformUserRecord(ctx, userID)
}

func (s *Store) CreatePlatformUser(ctx context.Context, username, email, displayName, passwordHash string, roleCodes []string, createdBy *string) (domain.PlatformUserRecord, error) {
	username = normalizeUsername(username)
	email = normalizeEmail(email)
	displayName = strings.TrimSpace(displayName)
	if username == "" || strings.TrimSpace(passwordHash) == "" {
		return domain.PlatformUserRecord{}, errors.New("username and password hash are required")
	}
	if email == "" {
		email = username + "@rtis.local"
	}
	if displayName == "" {
		displayName = username
	}
	if len(roleCodes) == 0 {
		roleCodes = []string{"readonly"}
	}

	tx, err := s.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return domain.PlatformUserRecord{}, err
	}
	defer tx.Rollback(ctx)

	user := domain.PlatformUserRecord{
		PlatformUser: domain.PlatformUser{
			ID:          id.New(),
			Username:    username,
			Email:       email,
			DisplayName: displayName,
			Status:      "active",
			AuthSource:  "local",
			CreatedAt:   time.Now().UTC(),
			UpdatedAt:   time.Now().UTC(),
		},
		RoleCodes: make([]string, 0, len(roleCodes)),
	}
	if _, err := tx.Exec(ctx, `insert into platform_users(id,username,email,display_name,status,password_hash,auth_source,created_at,updated_at) values($1,$2,$3,$4,$5,$6,$7,$8,$9)`,
		user.ID, user.Username, user.Email, user.DisplayName, user.Status, passwordHash, user.AuthSource, user.CreatedAt, user.UpdatedAt); err != nil {
		return domain.PlatformUserRecord{}, err
	}

	for _, roleCode := range roleCodes {
		roleCode = strings.TrimSpace(roleCode)
		if roleCode == "" {
			continue
		}
		var roleID string
		if err := tx.QueryRow(ctx, `select id from roles where code=$1`, roleCode).Scan(&roleID); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return domain.PlatformUserRecord{}, errors.New("unknown role: " + roleCode)
			}
			return domain.PlatformUserRecord{}, err
		}
		if _, err := tx.Exec(ctx, `insert into platform_user_roles(user_id,role_id,assigned_by,created_at) values($1,$2,$3,now()) on conflict(user_id, role_id) do nothing`,
			user.ID, roleID, createdBy); err != nil {
			return domain.PlatformUserRecord{}, err
		}
		user.RoleCodes = append(user.RoleCodes, roleCode)
	}

	if err := tx.Commit(ctx); err != nil {
		return domain.PlatformUserRecord{}, err
	}
	_, _ = s.CreateAuditForUser(ctx, createdBy, "auth.user.create", "platform_user", &user.ID, "platform user created")
	return user, nil
}

func (s *Store) UpdatePlatformUserStatus(ctx context.Context, userID, status string, updatedBy *string) (domain.PlatformUserRecord, error) {
	userID = strings.TrimSpace(userID)
	status = strings.TrimSpace(status)
	if userID == "" {
		return domain.PlatformUserRecord{}, errors.New("user id is required")
	}
	if status != "active" && status != "disabled" && status != "locked" {
		return domain.PlatformUserRecord{}, errors.New("invalid platform user status")
	}
	tag, err := s.db.Exec(ctx, `update platform_users set status=$2,updated_at=now() where id=$1`, userID, status)
	if err != nil {
		return domain.PlatformUserRecord{}, err
	}
	if tag.RowsAffected() == 0 {
		return domain.PlatformUserRecord{}, errors.New("platform user not found")
	}
	x, err := s.getPlatformUserRecord(ctx, userID)
	if err != nil {
		return domain.PlatformUserRecord{}, err
	}
	_, _ = s.CreateAuditForUser(ctx, updatedBy, "auth.user.status", "platform_user", &userID, "platform user status set to "+status)
	return x, nil
}

func (s *Store) UpdatePlatformUserPassword(ctx context.Context, userID, passwordHash string, updatedBy *string) error {
	userID = strings.TrimSpace(userID)
	if userID == "" || strings.TrimSpace(passwordHash) == "" {
		return errors.New("user id and password hash are required")
	}
	tag, err := s.db.Exec(ctx, `update platform_users set password_hash=$2,updated_at=now() where id=$1`, userID, passwordHash)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return errors.New("platform user not found")
	}
	_, _ = s.CreateAuditForUser(ctx, updatedBy, "auth.user.password", "platform_user", &userID, "platform user password updated")
	return nil
}

func (s *Store) DeletePlatformUser(ctx context.Context, userID string, deletedBy *string) error {
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return errors.New("user id is required")
	}
	if deletedBy != nil && strings.TrimSpace(*deletedBy) == userID {
		return errors.New("cannot delete the current operator")
	}

	record, err := s.getPlatformUserRecord(ctx, userID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return errors.New("platform user not found")
		}
		return err
	}

	tx, err := s.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	if hasRoleCode(record.RoleCodes, "superadmin") {
		var remaining int
		if err := tx.QueryRow(ctx, `select count(distinct pur.user_id)
			from platform_user_roles pur
			join roles r on r.id=pur.role_id
			where r.code='superadmin' and pur.user_id <> $1`, userID).Scan(&remaining); err != nil {
			return err
		}
		if remaining == 0 {
			return errors.New("cannot delete the last superadmin")
		}
	}

	if _, err := tx.Exec(ctx, `delete from platform_users where id=$1`, userID); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	}
	_, _ = s.CreateAuditForUser(ctx, deletedBy, "auth.user.delete", "platform_user", &userID, "platform user deleted")
	return nil
}

func (s *Store) ListUserSessions(ctx context.Context, limit int) ([]domain.UserSessionRecord, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	rows, err := s.db.Query(ctx, `select
		s.id,
		s.user_id,
		s.expires_at,
		s.revoked_at,
		s.created_at,
		coalesce(host(s.ip),''),
		coalesce(s.user_agent,''),
		u.username,
		u.email,
		u.display_name
	from user_sessions s
	join platform_users u on u.id=s.user_id
	where s.revoked_at is null
	order by s.created_at desc
	limit $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.UserSessionRecord{}
	for rows.Next() {
		var x domain.UserSessionRecord
		if err := rows.Scan(&x.ID, &x.UserID, &x.ExpiresAt, &x.RevokedAt, &x.CreatedAt, &x.IP, &x.UserAgent, &x.Username, &x.Email, &x.DisplayName); err != nil {
			return nil, err
		}
		out = append(out, x)
	}
	return out, rows.Err()
}

func (s *Store) TouchPlatformUserLogin(ctx context.Context, userID string) error {
	_, err := s.db.Exec(ctx, `update platform_users set last_login_at=now(),updated_at=now() where id=$1`, userID)
	return err
}

func (s *Store) CreateUserSession(ctx context.Context, userID, tokenHash, ipAddr, userAgent string, expiresAt time.Time) (domain.UserSession, error) {
	x := domain.UserSession{
		ID:        id.New(),
		UserID:    userID,
		ExpiresAt: expiresAt,
		CreatedAt: time.Now().UTC(),
	}
	_, err := s.db.Exec(ctx, `insert into user_sessions(id,user_id,session_token_hash,ip,user_agent,expires_at,created_at) values($1,$2,$3,nullif($4,'')::inet,nullif($5,''),$6,$7)`,
		x.ID, x.UserID, tokenHash, strings.TrimSpace(ipAddr), strings.TrimSpace(userAgent), x.ExpiresAt, x.CreatedAt)
	return x, err
}

func (s *Store) ResolveAuthContext(ctx context.Context, tokenHash string) (domain.AuthContext, error) {
	var out domain.AuthContext
	row := s.db.QueryRow(ctx, `select
		s.id,
		s.user_id,
		s.expires_at,
		s.revoked_at,
		s.created_at,
		u.id,
		u.username,
		u.email,
		u.display_name,
		u.status,
		u.auth_source,
		u.last_login_at,
		u.created_at,
		u.updated_at
	from user_sessions s
	join platform_users u on u.id=s.user_id
	where s.session_token_hash=$1
	  and s.revoked_at is null
	  and s.expires_at > now()
	  and u.status='active'`, tokenHash)
	err := row.Scan(
		&out.Session.ID,
		&out.Session.UserID,
		&out.Session.ExpiresAt,
		&out.Session.RevokedAt,
		&out.Session.CreatedAt,
		&out.User.ID,
		&out.User.Username,
		&out.User.Email,
		&out.User.DisplayName,
		&out.User.Status,
		&out.User.AuthSource,
		&out.User.LastLoginAt,
		&out.User.CreatedAt,
		&out.User.UpdatedAt,
	)
	if err != nil {
		return out, err
	}

	roleRows, err := s.db.Query(ctx, `select r.code
		from roles r
		join platform_user_roles pur on pur.role_id=r.id
		where pur.user_id=$1
		order by r.code`, out.User.ID)
	if err != nil {
		return out, err
	}
	defer roleRows.Close()
	for roleRows.Next() {
		var code string
		if err := roleRows.Scan(&code); err != nil {
			return out, err
		}
		out.RoleCodes = append(out.RoleCodes, code)
	}
	if err := roleRows.Err(); err != nil {
		return out, err
	}

	permRows, err := s.db.Query(ctx, `select distinct p.code
		from permissions p
		join role_permissions rp on rp.permission_id=p.id
		join platform_user_roles pur on pur.role_id=rp.role_id
		where pur.user_id=$1
		order by p.code`, out.User.ID)
	if err != nil {
		return out, err
	}
	defer permRows.Close()
	for permRows.Next() {
		var code string
		if err := permRows.Scan(&code); err != nil {
			return out, err
		}
		out.PermissionCodes = append(out.PermissionCodes, code)
	}
	return out, permRows.Err()
}

func (s *Store) RevokeUserSession(ctx context.Context, sessionID string) error {
	_, err := s.db.Exec(ctx, `update user_sessions set revoked_at=now() where id=$1 and revoked_at is null`, sessionID)
	return err
}

func (s *Store) RevokeUserSessionsByUser(ctx context.Context, userID string) error {
	_, err := s.db.Exec(ctx, `update user_sessions set revoked_at=now() where user_id=$1 and revoked_at is null`, strings.TrimSpace(userID))
	return err
}

func (s *Store) CreateAuditForUser(ctx context.Context, userID *string, action, resource string, resourceID *string, summary string) (domain.AuditEvent, error) {
	a := domain.AuditEvent{
		ID:           id.New(),
		ActorUserID:  userID,
		ActorType:    "platform_user",
		Action:       action,
		ResourceType: resource,
		ResourceID:   resourceID,
		Summary:      summary,
		CreatedAt:    time.Now().UTC(),
	}
	_, err := s.db.Exec(ctx, `insert into audit_events(id,actor_user_id,actor_type,action,resource_type,resource_id,summary,payload_json,created_at) values($1,$2,$3,$4,$5,$6,$7,'{}'::jsonb,$8)`,
		a.ID, a.ActorUserID, a.ActorType, a.Action, a.ResourceType, a.ResourceID, a.Summary, a.CreatedAt)
	return a, err
}

func normalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

func normalizeUsername(username string) string {
	return strings.ToLower(strings.TrimSpace(username))
}

func (s *Store) listPlatformUserRoles(ctx context.Context, userID string) ([]string, error) {
	rows, err := s.db.Query(ctx, `select r.code
		from roles r
		join platform_user_roles pur on pur.role_id=r.id
		where pur.user_id=$1
		order by r.code`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []string{}
	for rows.Next() {
		var code string
		if err := rows.Scan(&code); err != nil {
			return nil, err
		}
		out = append(out, code)
	}
	return out, rows.Err()
}

func (s *Store) getPlatformUserRecord(ctx context.Context, userID string) (domain.PlatformUserRecord, error) {
	var out domain.PlatformUserRecord
	err := s.db.QueryRow(ctx, `select id,username,email,display_name,status,auth_source,last_login_at,created_at,updated_at from platform_users where id=$1`, strings.TrimSpace(userID)).
		Scan(&out.ID, &out.Username, &out.Email, &out.DisplayName, &out.Status, &out.AuthSource, &out.LastLoginAt, &out.CreatedAt, &out.UpdatedAt)
	if err != nil {
		return out, err
	}
	out.RoleCodes, err = s.listPlatformUserRoles(ctx, out.ID)
	return out, err
}

func hasRoleCode(roleCodes []string, target string) bool {
	target = strings.TrimSpace(target)
	for _, roleCode := range roleCodes {
		if strings.TrimSpace(roleCode) == target {
			return true
		}
	}
	return false
}
