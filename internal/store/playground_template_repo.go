package store

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/APICerberus/APICerebrus/internal/pkg/uuid"
)

type PlaygroundTemplate struct {
	ID        string            `json:"id"`
	UserID    string            `json:"user_id"`
	Name      string            `json:"name"`
	Method    string            `json:"method"`
	Path      string            `json:"path"`
	Headers   map[string]string `json:"headers"`
	Query     map[string]string `json:"query"`
	Body      string            `json:"body"`
	CreatedAt time.Time         `json:"created_at"`
	UpdatedAt time.Time         `json:"updated_at"`
}

type PlaygroundTemplateRepo struct {
	db  *sql.DB
	now func() time.Time
}

func (s *Store) PlaygroundTemplates() *PlaygroundTemplateRepo {
	if s == nil || s.db == nil {
		return nil
	}
	return &PlaygroundTemplateRepo{
		db:  s.db,
		now: time.Now,
	}
}

func (r *PlaygroundTemplateRepo) ListByUser(userID string) ([]PlaygroundTemplate, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("playground template repo is not initialized")
	}
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return nil, errors.New("user id is required")
	}

	rows, err := r.db.Query(`
		SELECT id, user_id, name, method, path, headers, query_params, body, created_at, updated_at
		  FROM playground_templates
		 WHERE user_id = ?
		 ORDER BY updated_at DESC
	`, userID)
	if err != nil {
		return nil, fmt.Errorf("list playground templates: %w", err)
	}
	defer rows.Close()

	templates := make([]PlaygroundTemplate, 0, 32)
	for rows.Next() {
		item, err := scanPlaygroundTemplateRows(rows)
		if err != nil {
			return nil, err
		}
		templates = append(templates, *item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate playground templates: %w", err)
	}
	return templates, nil
}

func (r *PlaygroundTemplateRepo) Save(template *PlaygroundTemplate) error {
	if r == nil || r.db == nil {
		return errors.New("playground template repo is not initialized")
	}
	if template == nil {
		return errors.New("template is nil")
	}
	template.UserID = strings.TrimSpace(template.UserID)
	template.Name = strings.TrimSpace(template.Name)
	template.Method = strings.ToUpper(strings.TrimSpace(template.Method))
	template.Path = strings.TrimSpace(template.Path)
	if template.UserID == "" {
		return errors.New("template user id is required")
	}
	if template.Name == "" {
		return errors.New("template name is required")
	}
	if template.Method == "" {
		template.Method = "GET"
	}
	if template.Path == "" {
		template.Path = "/"
	}
	if template.Headers == nil {
		template.Headers = map[string]string{}
	}
	if template.Query == nil {
		template.Query = map[string]string{}
	}

	headersJSON, err := marshalStringMap(template.Headers)
	if err != nil {
		return err
	}
	queryJSON, err := marshalStringMap(template.Query)
	if err != nil {
		return err
	}

	now := r.now().UTC()
	if template.ID == "" {
		id, err := uuid.NewString()
		if err != nil {
			return err
		}
		template.ID = id
		template.CreatedAt = now
		template.UpdatedAt = now
		_, err = r.db.Exec(`
			INSERT INTO playground_templates(
				id, user_id, name, method, path, headers, query_params, body, created_at, updated_at
			) VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		`,
			template.ID,
			template.UserID,
			template.Name,
			template.Method,
			template.Path,
			headersJSON,
			queryJSON,
			template.Body,
			template.CreatedAt.Format(time.RFC3339Nano),
			template.UpdatedAt.Format(time.RFC3339Nano),
		)
		if err != nil {
			return fmt.Errorf("create playground template: %w", err)
		}
		return nil
	}

	template.UpdatedAt = now
	result, err := r.db.Exec(`
		UPDATE playground_templates
		   SET name = ?, method = ?, path = ?, headers = ?, query_params = ?, body = ?, updated_at = ?
		 WHERE id = ? AND user_id = ?
	`,
		template.Name,
		template.Method,
		template.Path,
		headersJSON,
		queryJSON,
		template.Body,
		template.UpdatedAt.Format(time.RFC3339Nano),
		template.ID,
		template.UserID,
	)
	if err != nil {
		return fmt.Errorf("update playground template: %w", err)
	}
	affected, _ := result.RowsAffected()
	if affected == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (r *PlaygroundTemplateRepo) DeleteForUser(id, userID string) error {
	if r == nil || r.db == nil {
		return errors.New("playground template repo is not initialized")
	}
	id = strings.TrimSpace(id)
	userID = strings.TrimSpace(userID)
	if id == "" {
		return errors.New("template id is required")
	}
	if userID == "" {
		return errors.New("user id is required")
	}

	result, err := r.db.Exec(`DELETE FROM playground_templates WHERE id = ? AND user_id = ?`, id, userID)
	if err != nil {
		return fmt.Errorf("delete playground template: %w", err)
	}
	affected, _ := result.RowsAffected()
	if affected == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func scanPlaygroundTemplateRows(rows *sql.Rows) (*PlaygroundTemplate, error) {
	var (
		item                                      PlaygroundTemplate
		headersRaw, queryRaw, createdRaw, updated string
	)
	if err := rows.Scan(
		&item.ID,
		&item.UserID,
		&item.Name,
		&item.Method,
		&item.Path,
		&headersRaw,
		&queryRaw,
		&item.Body,
		&createdRaw,
		&updated,
	); err != nil {
		return nil, err
	}
	item.Method = strings.ToUpper(strings.TrimSpace(item.Method))
	if item.Method == "" {
		item.Method = "GET"
	}
	if item.Path == "" {
		item.Path = "/"
	}
	headers, err := unmarshalStringMap(headersRaw)
	if err != nil {
		return nil, fmt.Errorf("decode playground template headers: %w", err)
	}
	query, err := unmarshalStringMap(queryRaw)
	if err != nil {
		return nil, fmt.Errorf("decode playground template query: %w", err)
	}
	item.Headers = headers
	item.Query = query
	item.CreatedAt, err = time.Parse(time.RFC3339Nano, createdRaw)
	if err != nil {
		return nil, fmt.Errorf("decode playground template created_at: %w", err)
	}
	item.UpdatedAt, err = time.Parse(time.RFC3339Nano, updated)
	if err != nil {
		return nil, fmt.Errorf("decode playground template updated_at: %w", err)
	}
	return &item, nil
}

func marshalStringMap(value map[string]string) (string, error) {
	if value == nil {
		return "{}", nil
	}
	data, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func unmarshalStringMap(raw string) (map[string]string, error) {
	if strings.TrimSpace(raw) == "" {
		return map[string]string{}, nil
	}
	out := map[string]string{}
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return nil, err
	}
	return out, nil
}
