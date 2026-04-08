package store

import (
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/APICerberus/APICerebrus/internal/pkg/uuid"
)

var (
	ErrAPIKeyNotFound = errors.New("api key not found")
	ErrAPIKeyExpired  = errors.New("api key expired")
	ErrAPIKeyRevoked  = errors.New("api key revoked")
	ErrAPIKeyUserDown = errors.New("api key user is not active")
)

type APIKey struct {
	ID         string
	UserID     string
	KeyHash    string
	KeyPrefix  string
	Name       string
	Status     string
	ExpiresAt  *time.Time
	LastUsedAt *time.Time
	LastUsedIP string
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

type APIKeyRepo struct {
	db  *sql.DB
	now func() time.Time
}

func (s *Store) APIKeys() *APIKeyRepo {
	if s == nil || s.db == nil {
		return nil
	}
	return &APIKeyRepo{
		db:  s.db,
		now: time.Now,
	}
}

func (r *APIKeyRepo) Create(userID, name, mode string) (string, *APIKey, error) {
	if r == nil || r.db == nil {
		return "", nil, errors.New("api key repo is not initialized")
	}
	userID = strings.TrimSpace(userID)
	name = strings.TrimSpace(name)
	if userID == "" {
		return "", nil, errors.New("user id is required")
	}
	if name == "" {
		name = "default"
	}

	var exists int
	if err := r.db.QueryRow(`SELECT COUNT(*) FROM users WHERE id = ? AND status != 'deleted'`, userID).Scan(&exists); err != nil {
		return "", nil, fmt.Errorf("check user for api key create: %w", err)
	}
	if exists == 0 {
		return "", nil, sql.ErrNoRows
	}

	prefix := "ck_live_"
	if strings.EqualFold(strings.TrimSpace(mode), "test") {
		prefix = "ck_test_"
	}
	token, err := randomToken(32)
	if err != nil {
		return "", nil, err
	}
	fullKey := prefix + token
	hash := hashAPIKey(fullKey)
	keyPrefix := fullKey
	if len(keyPrefix) > 12 {
		keyPrefix = keyPrefix[:12]
	}

	id, err := uuid.NewString()
	if err != nil {
		return "", nil, err
	}
	now := r.now().UTC()

	_, err = r.db.Exec(`
		INSERT INTO api_keys(
			id, user_id, key_hash, key_prefix, name, status,
			expires_at, last_used_at, last_used_ip, created_at, updated_at
		) VALUES(?, ?, ?, ?, ?, 'active', '', '', '', ?, ?)
	`,
		id,
		userID,
		hash,
		keyPrefix,
		name,
		now.Format(time.RFC3339Nano),
		now.Format(time.RFC3339Nano),
	)
	if err != nil {
		return "", nil, fmt.Errorf("insert api key: %w", err)
	}

	key := &APIKey{
		ID:         id,
		UserID:     userID,
		KeyHash:    hash,
		KeyPrefix:  keyPrefix,
		Name:       name,
		Status:     "active",
		LastUsedIP: "",
		CreatedAt:  now,
		UpdatedAt:  now,
	}
	return fullKey, key, nil
}

func (r *APIKeyRepo) FindByHash(hash string) (*APIKey, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("api key repo is not initialized")
	}
	hash = strings.TrimSpace(strings.ToLower(hash))
	if hash == "" {
		return nil, errors.New("api key hash is required")
	}

	row := r.db.QueryRow(`
		SELECT id, user_id, key_hash, key_prefix, name, status,
		       expires_at, last_used_at, last_used_ip, created_at, updated_at
		  FROM api_keys
		 WHERE key_hash = ?
	`, hash)
	key, err := scanAPIKey(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return key, nil
}

func (r *APIKeyRepo) ListByUser(userID string) ([]APIKey, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("api key repo is not initialized")
	}
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return nil, errors.New("user id is required")
	}

	rows, err := r.db.Query(`
		SELECT id, user_id, key_hash, key_prefix, name, status,
		       expires_at, last_used_at, last_used_ip, created_at, updated_at
		  FROM api_keys
		 WHERE user_id = ?
		 ORDER BY created_at DESC
	`, userID)
	if err != nil {
		return nil, fmt.Errorf("list api keys: %w", err)
	}
	defer rows.Close()

	out := make([]APIKey, 0)
	for rows.Next() {
		key, err := scanAPIKeyRows(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *key)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate api keys: %w", err)
	}
	return out, nil
}

func (r *APIKeyRepo) Revoke(id string) error {
	if r == nil || r.db == nil {
		return errors.New("api key repo is not initialized")
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return errors.New("api key id is required")
	}
	result, err := r.db.Exec(`UPDATE api_keys SET status='revoked', updated_at=? WHERE id = ?`, r.now().UTC().Format(time.RFC3339Nano), id)
	if err != nil {
		return fmt.Errorf("revoke api key: %w", err)
	}
	affected, _ := result.RowsAffected()
	if affected == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (r *APIKeyRepo) RenameForUser(id, userID, name string) error {
	if r == nil || r.db == nil {
		return errors.New("api key repo is not initialized")
	}
	id = strings.TrimSpace(id)
	userID = strings.TrimSpace(userID)
	name = strings.TrimSpace(name)
	if id == "" {
		return errors.New("api key id is required")
	}
	if userID == "" {
		return errors.New("user id is required")
	}
	if name == "" {
		return errors.New("api key name is required")
	}

	result, err := r.db.Exec(
		`UPDATE api_keys SET name = ?, updated_at = ? WHERE id = ? AND user_id = ?`,
		name,
		r.now().UTC().Format(time.RFC3339Nano),
		id,
		userID,
	)
	if err != nil {
		return fmt.Errorf("rename api key: %w", err)
	}
	affected, _ := result.RowsAffected()
	if affected == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (r *APIKeyRepo) RevokeForUser(id, userID string) error {
	if r == nil || r.db == nil {
		return errors.New("api key repo is not initialized")
	}
	id = strings.TrimSpace(id)
	userID = strings.TrimSpace(userID)
	if id == "" {
		return errors.New("api key id is required")
	}
	if userID == "" {
		return errors.New("user id is required")
	}

	result, err := r.db.Exec(
		`UPDATE api_keys SET status='revoked', updated_at=? WHERE id = ? AND user_id = ?`,
		r.now().UTC().Format(time.RFC3339Nano),
		id,
		userID,
	)
	if err != nil {
		return fmt.Errorf("revoke api key: %w", err)
	}
	affected, _ := result.RowsAffected()
	if affected == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (r *APIKeyRepo) UpdateLastUsed(id, ip string) {
	if r == nil || r.db == nil {
		return
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return
	}
	go func() {
		if _, err := r.db.Exec(
			`UPDATE api_keys SET last_used_at = ?, last_used_ip = ?, updated_at = ? WHERE id = ?`,
			r.now().UTC().Format(time.RFC3339Nano),
			strings.TrimSpace(ip),
			r.now().UTC().Format(time.RFC3339Nano),
			id,
		); err != nil {
			log.Printf("[ERROR] api_key_repo: failed to update last_used for key %s: %v", id, err)
		}
	}()
}

func (r *APIKeyRepo) ResolveUserByRawKey(raw string) (*User, *APIKey, error) {
	if r == nil || r.db == nil {
		return nil, nil, errors.New("api key repo is not initialized")
	}
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil, ErrAPIKeyNotFound
	}

	keyHash := hashAPIKey(raw)
	row := r.db.QueryRow(`
		SELECT k.id, k.user_id, k.key_hash, k.key_prefix, k.name, k.status,
		       k.expires_at, k.last_used_at, k.last_used_ip, k.created_at, k.updated_at,
		       u.id, u.email, u.name, u.company, u.password_hash, u.role, u.status,
		       u.credit_balance, u.rate_limits, u.ip_whitelist, u.metadata, u.created_at, u.updated_at
		  FROM api_keys k
		  JOIN users u ON u.id = k.user_id
		 WHERE k.key_hash = ?
		 LIMIT 1
	`, keyHash)

	var (
		key                                          APIKey
		keyExpiresRaw, keyLastUsedRaw                string
		keyCreatedRaw, keyUpdatedRaw                 string
		user                                         User
		userRateLimitsRaw, userIPWhitelistRaw        string
		userMetadataRaw, userCreatedRaw, userUpdated string
	)

	err := row.Scan(
		&key.ID, &key.UserID, &key.KeyHash, &key.KeyPrefix, &key.Name, &key.Status,
		&keyExpiresRaw, &keyLastUsedRaw, &key.LastUsedIP, &keyCreatedRaw, &keyUpdatedRaw,
		&user.ID, &user.Email, &user.Name, &user.Company, &user.PasswordHash, &user.Role, &user.Status,
		&user.CreditBalance, &userRateLimitsRaw, &userIPWhitelistRaw, &userMetadataRaw, &userCreatedRaw, &userUpdated,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil, ErrAPIKeyNotFound
		}
		return nil, nil, fmt.Errorf("resolve api key: %w", err)
	}

	if err := decodeAPIKeyDateFields(&key, keyExpiresRaw, keyLastUsedRaw, keyCreatedRaw, keyUpdatedRaw); err != nil {
		return nil, nil, err
	}
	if err := decodeUserJSONFields(&user, userRateLimitsRaw, userIPWhitelistRaw, userMetadataRaw, userCreatedRaw, userUpdated); err != nil {
		return nil, nil, err
	}

	if strings.EqualFold(key.Status, "revoked") {
		return nil, nil, ErrAPIKeyRevoked
	}
	if strings.EqualFold(user.Status, "suspended") || strings.EqualFold(user.Status, "deleted") {
		return nil, nil, ErrAPIKeyUserDown
	}
	if key.ExpiresAt != nil && time.Now().After(*key.ExpiresAt) {
		return nil, nil, ErrAPIKeyExpired
	}
	return &user, &key, nil
}

func hashAPIKey(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

func randomToken(length int) (string, error) {
	if length <= 0 {
		return "", errors.New("token length must be positive")
	}
	const alphabet = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	raw := make([]byte, length)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("generate api key token: %w", err)
	}
	out := make([]byte, length)
	for i := range raw {
		out[i] = alphabet[int(raw[i])%len(alphabet)]
	}
	return string(out), nil
}

func scanAPIKey(row *sql.Row) (*APIKey, error) {
	var (
		key                                          APIKey
		expiresRaw, lastUsedRaw, createdRaw, updated string
	)
	if err := row.Scan(
		&key.ID, &key.UserID, &key.KeyHash, &key.KeyPrefix, &key.Name, &key.Status,
		&expiresRaw, &lastUsedRaw, &key.LastUsedIP, &createdRaw, &updated,
	); err != nil {
		return nil, err
	}
	if err := decodeAPIKeyDateFields(&key, expiresRaw, lastUsedRaw, createdRaw, updated); err != nil {
		return nil, err
	}
	return &key, nil
}

func scanAPIKeyRows(rows *sql.Rows) (*APIKey, error) {
	var (
		key                                          APIKey
		expiresRaw, lastUsedRaw, createdRaw, updated string
	)
	if err := rows.Scan(
		&key.ID, &key.UserID, &key.KeyHash, &key.KeyPrefix, &key.Name, &key.Status,
		&expiresRaw, &lastUsedRaw, &key.LastUsedIP, &createdRaw, &updated,
	); err != nil {
		return nil, err
	}
	if err := decodeAPIKeyDateFields(&key, expiresRaw, lastUsedRaw, createdRaw, updated); err != nil {
		return nil, err
	}
	return &key, nil
}

func decodeAPIKeyDateFields(key *APIKey, expiresRaw, lastUsedRaw, createdRaw, updatedRaw string) error {
	if key == nil {
		return errors.New("api key is nil")
	}
	if strings.TrimSpace(expiresRaw) != "" {
		expiresAt, err := time.Parse(time.RFC3339Nano, expiresRaw)
		if err != nil {
			return fmt.Errorf("decode api key expires_at: %w", err)
		}
		key.ExpiresAt = &expiresAt
	}
	if strings.TrimSpace(lastUsedRaw) != "" {
		lastUsedAt, err := time.Parse(time.RFC3339Nano, lastUsedRaw)
		if err != nil {
			return fmt.Errorf("decode api key last_used_at: %w", err)
		}
		key.LastUsedAt = &lastUsedAt
	}
	createdAt, err := time.Parse(time.RFC3339Nano, createdRaw)
	if err != nil {
		return fmt.Errorf("decode api key created_at: %w", err)
	}
	updatedAt, err := time.Parse(time.RFC3339Nano, updatedRaw)
	if err != nil {
		return fmt.Errorf("decode api key updated_at: %w", err)
	}
	key.CreatedAt = createdAt
	key.UpdatedAt = updatedAt
	return nil
}
