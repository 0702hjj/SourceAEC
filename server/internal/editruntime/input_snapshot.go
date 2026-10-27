// SPDX-License-Identifier: Apache-2.0
package editruntime

import (
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"path"
	"regexp"
	"strings"
)

const buildInputLimit = 64 << 20
const buildInputsLimit = 256 << 20

var inputProjectPattern = regexp.MustCompile(`^p_[a-zA-Z0-9]+$`)

// Use one statement so project ownership and input bytes share a snapshot,
// even in a read-committed editing transaction. Reuse the caller's connection:
// opening another transaction could deadlock a pool full of editing requests.
func collectInputs(ctx context.Context, db queryer, model string) (map[string]string, error) {
	if model == "" || model == "." || model == ".." || strings.ContainsAny(model, "/\\") {
		return nil, errors.New("invalid input model")
	}
	rows, err := db.QueryContext(ctx, `
WITH owner AS MATERIALIZED (
    SELECT content, convert_from(content,'UTF8')::jsonb->>'projectId' AS project
    FROM artifacts WHERE path=$1
)
SELECT owner.content, input.path, octet_length(input.content),
       CASE WHEN octet_length(input.content)<=$2 THEN input.content END
FROM owner LEFT JOIN artifacts input ON
    (starts_with(input.path,'plans/' || owner.project || '/') OR
     starts_with(input.path,'skill-work/' || owner.project || '/')) AND
    (right(input.path,5)='.json' OR right(input.path,3)='.py' OR right(input.path,4)='.dxf')
WHERE owner.project IS NOT NULL`, "models/"+model+"/model.json", buildInputLimit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := map[string]string{}
	var total int64
	for rows.Next() {
		var metadata, content []byte
		var key sql.NullString
		var size sql.NullInt64
		if err = rows.Scan(&metadata, &key, &size, &content); err != nil {
			return nil, err
		}
		var owner struct {
			Project string `json:"projectId"`
		}
		if err = json.Unmarshal(metadata, &owner); err != nil {
			return nil, err
		}
		if !inputProjectPattern.MatchString(owner.Project) || !key.Valid {
			continue
		}
		include, err := validateInputKey(key.String, owner.Project)
		if err != nil {
			return nil, err
		}
		if !include {
			continue
		}
		total += size.Int64
		if err = validateInputSize(size.Int64, total); err != nil {
			return nil, err
		}
		result[key.String] = base64.StdEncoding.EncodeToString(content)
	}
	return result, rows.Err()
}

func validateInputKey(key, project string) (bool, error) {
	if path.Clean(key) != key || strings.Contains(key, "\\") || (!strings.HasPrefix(key, "plans/"+project+"/") && !strings.HasPrefix(key, "skill-work/"+project+"/")) {
		return false, errors.New("invalid build input path")
	}
	for _, part := range strings.Split(key, "/") {
		if part == "plan_history" {
			return false, nil
		}
	}
	ext := path.Ext(key)
	return ext == ".json" || ext == ".py" || ext == ".dxf", nil
}

func validateInputSize(size, total int64) error {
	if size > buildInputLimit {
		return errors.New("build input exceeds 64 MiB")
	}
	if total > buildInputsLimit {
		return errors.New("build inputs exceed 256 MiB")
	}
	return nil
}
