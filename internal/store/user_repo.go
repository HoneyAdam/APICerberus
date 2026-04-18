package store

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/APICerberus/APICerebrus/internal/pkg/uuid"
	"golang.org/x/crypto/bcrypt"
)

type User struct {
	ID            string
	Email         string
	Name          string
	Company       string
	PasswordHash  string
	Role          string
	Status        string
	CreditBalance int64
	RateLimits    map[string]any
	IPWhitelist   []string
	Metadata      map[string]any
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

type UserListOptions struct {
	Search   string
	Status   string
	Role     string
	SortBy   string
	SortDesc bool
	Limit    int
	Offset   int
}

type UserListResult struct {
	Users []User
	Total int
}

type UserRepo struct {
	db  DB
	now func() time.Time
}

var ErrInsufficientCredits = errors.New("insufficient credits")

func (s *Store) Users() *UserRepo {
	if s == nil || s.db == nil {
		return nil
	}
	return &UserRepo{
		db:  s.db,
		now: time.Now,
	}
}

func (r *UserRepo) Create(user *User) error {
	if r == nil || r.db == nil {
		return errors.New("user repo is not initialized")
	}
	if user == nil {
		return errors.New("user is nil")
	}
	if err := validateUserInput(*user); err != nil {
		return err
	}

	if strings.TrimSpace(user.ID) == "" {
		id, err := uuid.NewString()
		if err != nil {
			return err
		}
		user.ID = id
	}
	if strings.TrimSpace(user.Role) == "" {
		user.Role = "user"
	}
	if strings.TrimSpace(user.Status) == "" {
		user.Status = "active"
	}

	now := r.now().UTC()
	if user.CreatedAt.IsZero() {
		user.CreatedAt = now
	}
	user.UpdatedAt = now

	rateLimits, err := marshalJSON(user.RateLimits, "{}")
	if err != nil {
		return err
	}
	ipWhitelist, err := marshalJSON(user.IPWhitelist, "[]")
	if err != nil {
		return err
	}
	metadata, err := marshalJSON(user.Metadata, "{}")
	if err != nil {
		return err
	}

	_, err = r.db.Exec(`
		INSERT INTO users(
			id, email, name, company, password_hash, role, status,
			credit_balance, rate_limits, ip_whitelist, metadata, created_at, updated_at
		) VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`,
		user.ID,
		strings.TrimSpace(strings.ToLower(user.Email)),
		strings.TrimSpace(user.Name),
		strings.TrimSpace(user.Company),
		strings.TrimSpace(user.PasswordHash),
		strings.TrimSpace(strings.ToLower(user.Role)),
		strings.TrimSpace(strings.ToLower(user.Status)),
		user.CreditBalance,
		rateLimits,
		ipWhitelist,
		metadata,
		user.CreatedAt.UTC().Format(time.RFC3339Nano),
		user.UpdatedAt.UTC().Format(time.RFC3339Nano),
	)
	if err != nil {
		return fmt.Errorf("insert user: %w", err)
	}
	return nil
}

func (r *UserRepo) FindByID(id string) (*User, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("user repo is not initialized")
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return nil, errors.New("user id is required")
	}

	row := r.db.QueryRow(`
		SELECT id, email, name, company, password_hash, role, status,
		       credit_balance, rate_limits, ip_whitelist, metadata, created_at, updated_at
		  FROM users
		 WHERE id = ?
	`, id)
	user, err := scanUser(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return user, nil
}

func (r *UserRepo) FindByEmail(email string) (*User, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("user repo is not initialized")
	}
	email = strings.TrimSpace(strings.ToLower(email))
	if email == "" {
		return nil, errors.New("email is required")
	}

	row := r.db.QueryRow(`
		SELECT id, email, name, company, password_hash, role, status,
		       credit_balance, rate_limits, ip_whitelist, metadata, created_at, updated_at
		  FROM users
		 WHERE email = ?
	`, email)
	user, err := scanUser(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return user, nil
}

func (r *UserRepo) List(opts UserListOptions) (*UserListResult, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("user repo is not initialized")
	}

	where := make([]string, 0, 3)
	args := make([]any, 0, 6)

	if value := strings.TrimSpace(opts.Search); value != "" {
		where = append(where, "(LOWER(email) LIKE ? OR LOWER(name) LIKE ? OR LOWER(company) LIKE ?)")
		pattern := "%" + strings.ToLower(value) + "%"
		args = append(args, pattern, pattern, pattern)
	}
	if value := strings.TrimSpace(strings.ToLower(opts.Status)); value != "" {
		where = append(where, "status = ?")
		args = append(args, value)
	}
	if value := strings.TrimSpace(strings.ToLower(opts.Role)); value != "" {
		where = append(where, "role = ?")
		args = append(args, value)
	}

	whereSQL := ""
	if len(where) > 0 {
		whereSQL = " WHERE " + strings.Join(where, " AND ")
	}

	sortDir := "ASC"
	if opts.SortDesc {
		sortDir = "DESC"
	}

	limit := opts.Limit
	if limit <= 0 || limit > 500 {
		limit = 50
	}
	offset := opts.Offset
	if offset < 0 {
		offset = 0
	}

	countSQL := "SELECT COUNT(*) FROM users" + whereSQL
	var total int
	if err := r.db.QueryRow(countSQL, args...).Scan(&total); err != nil {
		return nil, fmt.Errorf("count users: %w", err)
	}

	query := fmt.Sprintf(`
		SELECT id, email, name, company, password_hash, role, status,
		       credit_balance, rate_limits, ip_whitelist, metadata, created_at, updated_at
		  FROM users%s
		 ORDER BY %s %s
		 LIMIT ? OFFSET ?`,
		whereSQL,                          // sanitized by callers
		normalizeUserSortBy(opts.SortBy), // M-GO-002: allowlist guard — only known column names reach this fmt
		sortDir)
	argsWithPage := append(append([]any(nil), args...), limit, offset)
	rows, err := r.db.Query(query, argsWithPage...)
	if err != nil {
		return nil, fmt.Errorf("list users: %w", err)
	}
	defer rows.Close()

	users := make([]User, 0, limit)
	for rows.Next() {
		user, err := scanUserRows(rows)
		if err != nil {
			return nil, err
		}
		users = append(users, *user)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate users: %w", err)
	}

	return &UserListResult{
		Users: users,
		Total: total,
	}, nil
}

func (r *UserRepo) Update(user *User) error {
	if r == nil || r.db == nil {
		return errors.New("user repo is not initialized")
	}
	if user == nil {
		return errors.New("user is nil")
	}
	user.ID = strings.TrimSpace(user.ID)
	if user.ID == "" {
		return errors.New("user id is required")
	}
	if err := validateUserInput(*user); err != nil {
		return err
	}

	if strings.TrimSpace(user.Role) == "" {
		user.Role = "user"
	}
	if strings.TrimSpace(user.Status) == "" {
		user.Status = "active"
	}
	user.UpdatedAt = r.now().UTC()

	rateLimits, err := marshalJSON(user.RateLimits, "{}")
	if err != nil {
		return err
	}
	ipWhitelist, err := marshalJSON(user.IPWhitelist, "[]")
	if err != nil {
		return err
	}
	metadata, err := marshalJSON(user.Metadata, "{}")
	if err != nil {
		return err
	}

	result, err := r.db.Exec(`
		UPDATE users
		   SET email = ?, name = ?, company = ?, password_hash = ?, role = ?, status = ?,
		       credit_balance = ?, rate_limits = ?, ip_whitelist = ?, metadata = ?, updated_at = ?
		 WHERE id = ?
	`,
		strings.TrimSpace(strings.ToLower(user.Email)),
		strings.TrimSpace(user.Name),
		strings.TrimSpace(user.Company),
		strings.TrimSpace(user.PasswordHash),
		strings.TrimSpace(strings.ToLower(user.Role)),
		strings.TrimSpace(strings.ToLower(user.Status)),
		user.CreditBalance,
		rateLimits,
		ipWhitelist,
		metadata,
		user.UpdatedAt.UTC().Format(time.RFC3339Nano),
		user.ID,
	)
	if err != nil {
		return fmt.Errorf("update user: %w", err)
	}
	affected, _ := result.RowsAffected()
	if affected == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (r *UserRepo) Delete(id string) error {
	if r == nil || r.db == nil {
		return errors.New("user repo is not initialized")
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return errors.New("user id is required")
	}

	result, err := r.db.Exec(`UPDATE users SET status = 'deleted', updated_at = ? WHERE id = ?`, r.now().UTC().Format(time.RFC3339Nano), id)
	if err != nil {
		return fmt.Errorf("soft delete user: %w", err)
	}
	affected, _ := result.RowsAffected()
	if affected == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (r *UserRepo) HardDelete(id string) error {
	if r == nil || r.db == nil {
		return errors.New("user repo is not initialized")
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return errors.New("user id is required")
	}
	result, err := r.db.Exec(`DELETE FROM users WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("hard delete user: %w", err)
	}
	affected, _ := result.RowsAffected()
	if affected == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (r *UserRepo) UpdateStatus(id, status string) error {
	if r == nil || r.db == nil {
		return errors.New("user repo is not initialized")
	}
	id = strings.TrimSpace(id)
	status = strings.TrimSpace(strings.ToLower(status))
	if id == "" {
		return errors.New("user id is required")
	}
	if status == "" {
		return errors.New("status is required")
	}

	result, err := r.db.Exec(`UPDATE users SET status = ?, updated_at = ? WHERE id = ?`, status, r.now().UTC().Format(time.RFC3339Nano), id)
	if err != nil {
		return fmt.Errorf("update user status: %w", err)
	}
	affected, _ := result.RowsAffected()
	if affected == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (r *UserRepo) UpdateRole(id, role string) error {
	if r == nil || r.db == nil {
		return errors.New("user repo is not initialized")
	}
	id = strings.TrimSpace(id)
	role = strings.TrimSpace(strings.ToLower(role))
	if id == "" {
		return errors.New("user id is required")
	}
	if role == "" {
		return errors.New("role is required")
	}

	result, err := r.db.Exec(`UPDATE users SET role = ?, updated_at = ? WHERE id = ?`, role, r.now().UTC().Format(time.RFC3339Nano), id)
	if err != nil {
		return fmt.Errorf("update user role: %w", err)
	}
	affected, _ := result.RowsAffected()
	if affected == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (r *UserRepo) UpdateCreditBalance(userID string, delta int64) (int64, error) {
	if r == nil || r.db == nil {
		return 0, errors.New("user repo is not initialized")
	}
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return 0, errors.New("user id is required")
	}

	var newBalance int64
	err := r.withTx(context.Background(), func(tx *sql.Tx) error {
		now := r.now().UTC().Format(time.RFC3339Nano)
		updateErr := tx.QueryRow(`
			UPDATE users
			   SET credit_balance = credit_balance + ?, updated_at = ?
			 WHERE id = ? AND credit_balance + ? >= 0
			RETURNING credit_balance
		`, delta, now, userID, delta).Scan(&newBalance)
		if updateErr == nil {
			return nil
		}
		if !errors.Is(updateErr, sql.ErrNoRows) {
			return fmt.Errorf("update credit balance: %w", updateErr)
		}

		var exists int
		if err := tx.QueryRow(`SELECT COUNT(*) FROM users WHERE id = ?`, userID).Scan(&exists); err != nil {
			return fmt.Errorf("check user for credit update: %w", err)
		}
		if exists == 0 {
			return sql.ErrNoRows
		}
		return ErrInsufficientCredits
	})
	if err != nil {
		return 0, err
	}
	return newBalance, nil
}

// UpdateCreditBalanceTx updates credit balance within an existing transaction.
func (r *UserRepo) UpdateCreditBalanceTx(tx *sql.Tx, userID string, delta int64) (int64, error) {
	if r == nil || tx == nil {
		return 0, errors.New("user repo or transaction is nil")
	}
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return 0, errors.New("user id is required")
	}

	var newBalance int64
	now := r.now().UTC().Format(time.RFC3339Nano)
	updateErr := tx.QueryRow(`
		UPDATE users
		   SET credit_balance = credit_balance + ?, updated_at = ?
		 WHERE id = ? AND credit_balance + ? >= 0
		RETURNING credit_balance
	`, delta, now, userID, delta).Scan(&newBalance)
	if updateErr == nil {
		return newBalance, nil
	}
	if !errors.Is(updateErr, sql.ErrNoRows) {
		return 0, fmt.Errorf("update credit balance: %w", updateErr)
	}

	var exists int
	if err := tx.QueryRow(`SELECT COUNT(*) FROM users WHERE id = ?`, userID).Scan(&exists); err != nil {
		return 0, fmt.Errorf("check user for credit update: %w", err)
	}
	if exists == 0 {
		return 0, sql.ErrNoRows
	}
	return 0, ErrInsufficientCredits
}

func HashPassword(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", errors.New("password is required")
	}

	// Use cost 12 for admin passwords (bcrypt.DefaultCost is 10, too low for admin).
	hash, err := bcrypt.GenerateFromPassword([]byte(raw), 12)
	if err != nil {
		return "", fmt.Errorf("hash password: %w", err)
	}
	return string(hash), nil
}

func VerifyPassword(stored, raw string) bool {
	stored = strings.TrimSpace(stored)
	raw = strings.TrimSpace(raw)
	if stored == "" || raw == "" {
		return false
	}

	err := bcrypt.CompareHashAndPassword([]byte(stored), []byte(raw))
	return err == nil
}

func (s *Store) ensureInitialAdminUser() error {
	if s == nil || s.db == nil {
		return errors.New("store is not initialized")
	}

	var count int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM users WHERE role = 'admin' AND status != 'deleted'`).Scan(&count); err != nil {
		return fmt.Errorf("count admin users: %w", err)
	}
	if count > 0 {
		return nil
	}

	// Get admin password from environment variable or generate a secure random one
	adminPassword := os.Getenv("APICERBERUS_ADMIN_PASSWORD")
	if adminPassword == "" {
		// Generate a secure random password if not set.
		// SECURITY: Never print the generated password to stdout/stderr as it may be
		// captured in log aggregation systems (ELK, Splunk, CloudWatch, etc.).
		// Instead, operators must set APICERBERUS_ADMIN_PASSWORD before startup.
		adminPassword = generateSecurePassword()
		fmt.Fprintf(os.Stderr, "WARNING: No APICERBERUS_ADMIN_PASSWORD env var set.\n")
		fmt.Fprintf(os.Stderr, "A temporary admin password was generated but cannot be shown here.\n")
		fmt.Fprintf(os.Stderr, "Set APICERBERUS_ADMIN_PASSWORD to a secure value before first login.\n")
		fmt.Fprintf(os.Stderr, "To recover: set the env var to a known value and restart the server.\n\n")
	}

	id, err := uuid.NewString()
	if err != nil {
		return err
	}
	passwordHash, err := HashPassword(adminPassword)
	if err != nil {
		return err
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err = s.db.Exec(`
		INSERT INTO users(
			id, email, name, company, password_hash, role, status,
			credit_balance, rate_limits, ip_whitelist, metadata, created_at, updated_at
		) VALUES(?, ?, ?, ?, ?, 'admin', 'active', 0, '{}', '[]', '{}', ?, ?)
		ON CONFLICT(email) DO UPDATE SET role='admin', status='active', updated_at=excluded.updated_at
	`,
		id,
		"admin@apicerberus.local",
		"Administrator",
		"",
		passwordHash,
		now,
		now,
	)
	if err != nil {
		return fmt.Errorf("ensure initial admin user: %w", err)
	}
	return nil
}

func generateSecurePassword() string {
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789!@#$%^&*"
	const length = 20
	charsetLen := len(charset)
	maxValid := 256 - (256 % charsetLen) // rejection sampling upper bound (CWE-330)

	password := make([]byte, length)
	buf := make([]byte, 1)
	for i := range password {
		for {
			if _, err := rand.Read(buf); err != nil {
				panic(fmt.Sprintf("crypto/rand unavailable: %v", err))
			}
			if int(buf[0]) < maxValid {
				password[i] = charset[int(buf[0])%charsetLen]
				break
			}
		}
	}
	return string(password)
}

func validateUserInput(user User) error {
	if strings.TrimSpace(user.Email) == "" {
		return errors.New("user email is required")
	}
	if !looksLikeEmail(strings.TrimSpace(user.Email)) {
		return errors.New("user email is invalid")
	}
	if strings.TrimSpace(user.Name) == "" {
		return errors.New("user name is required")
	}
	return nil
}

func looksLikeEmail(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" {
		return false
	}
	at := strings.Index(value, "@")
	if at <= 0 || at >= len(value)-1 {
		return false
	}
	domain := value[at+1:]
	return strings.Contains(domain, ".")
}

func normalizeUserSortBy(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "email":
		return "email"
	case "name":
		return "name"
	case "updated_at":
		return "updated_at"
	case "credit_balance":
		return "credit_balance"
	default:
		return "created_at"
	}
}

func marshalJSON(value any, fallback string) (string, error) {
	if value == nil {
		return fallback, nil
	}
	data, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func scanUser(row *sql.Row) (*User, error) {
	var (
		user                     User
		rateLimitsRaw            string
		ipWhitelistRaw           string
		metadataRaw              string
		createdAtRaw, updatedRaw string
	)
	if err := row.Scan(
		&user.ID,
		&user.Email,
		&user.Name,
		&user.Company,
		&user.PasswordHash,
		&user.Role,
		&user.Status,
		&user.CreditBalance,
		&rateLimitsRaw,
		&ipWhitelistRaw,
		&metadataRaw,
		&createdAtRaw,
		&updatedRaw,
	); err != nil {
		return nil, err
	}
	if err := decodeUserJSONFields(&user, rateLimitsRaw, ipWhitelistRaw, metadataRaw, createdAtRaw, updatedRaw); err != nil {
		return nil, err
	}
	return &user, nil
}

func scanUserRows(rows *sql.Rows) (*User, error) {
	var (
		user                     User
		rateLimitsRaw            string
		ipWhitelistRaw           string
		metadataRaw              string
		createdAtRaw, updatedRaw string
	)
	if err := rows.Scan(
		&user.ID,
		&user.Email,
		&user.Name,
		&user.Company,
		&user.PasswordHash,
		&user.Role,
		&user.Status,
		&user.CreditBalance,
		&rateLimitsRaw,
		&ipWhitelistRaw,
		&metadataRaw,
		&createdAtRaw,
		&updatedRaw,
	); err != nil {
		return nil, err
	}
	if err := decodeUserJSONFields(&user, rateLimitsRaw, ipWhitelistRaw, metadataRaw, createdAtRaw, updatedRaw); err != nil {
		return nil, err
	}
	return &user, nil
}

func decodeUserJSONFields(user *User, rateLimitsRaw, ipWhitelistRaw, metadataRaw, createdAtRaw, updatedRaw string) error {
	if user == nil {
		return errors.New("user is nil")
	}
	if strings.TrimSpace(rateLimitsRaw) == "" {
		rateLimitsRaw = "{}"
	}
	if strings.TrimSpace(ipWhitelistRaw) == "" {
		ipWhitelistRaw = "[]"
	}
	if strings.TrimSpace(metadataRaw) == "" {
		metadataRaw = "{}"
	}
	user.RateLimits = map[string]any{}
	if err := json.Unmarshal([]byte(rateLimitsRaw), &user.RateLimits); err != nil {
		return fmt.Errorf("decode user rate_limits: %w", err)
	}
	user.IPWhitelist = []string{}
	if err := json.Unmarshal([]byte(ipWhitelistRaw), &user.IPWhitelist); err != nil {
		return fmt.Errorf("decode user ip_whitelist: %w", err)
	}
	user.Metadata = map[string]any{}
	if err := json.Unmarshal([]byte(metadataRaw), &user.Metadata); err != nil {
		return fmt.Errorf("decode user metadata: %w", err)
	}

	createdAt, err := time.Parse(time.RFC3339Nano, createdAtRaw)
	if err != nil {
		return fmt.Errorf("decode user created_at: %w", err)
	}
	updatedAt, err := time.Parse(time.RFC3339Nano, updatedRaw)
	if err != nil {
		return fmt.Errorf("decode user updated_at: %w", err)
	}
	user.CreatedAt = createdAt
	user.UpdatedAt = updatedAt
	return nil
}

func (r *UserRepo) withTx(ctx context.Context, fn func(*sql.Tx) error) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	if err := fn(tx); err != nil {
		_ = tx.Rollback()
		return err
	}
	return tx.Commit()
}
