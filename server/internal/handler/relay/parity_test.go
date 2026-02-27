package relay

import (
	"bytes"
	"context"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/jimeng-relay/server/internal/middleware/sigv4"
	"github.com/jimeng-relay/server/internal/relay/upstream"
)

var relayAcceptedVideoReqKeys = map[string]struct{}{
	"jimeng_t2v_v30":            {},
	"jimeng_t2v_v30_1080p":           {},
	"jimeng_ti2v_v30_pro":            {},
	"jimeng_i2v_first_v30_1080":      {},
	"jimeng_i2v_first_tail_v30_1080": {},
	"jimeng_i2v_recamera_v30":        {},
}

func TestClientPresetMatrixParity_RelayContractCoverage(t *testing.T) {
	clientPresetReqKeys := mustLoadClientPresetReqKeys(t)

	if err := detectReqKeyParityMismatch(clientPresetReqKeys, relayAcceptedVideoReqKeys); err != nil {
		t.Fatalf("client/server req_key parity mismatch: %v", err)
	}
}

func TestClientPresetMatrixParity_SubmitPassthrough(t *testing.T) {
	clientPresetReqKeys := mustLoadClientPresetReqKeys(t)
	presets := make([]string, 0, len(clientPresetReqKeys))
	for preset := range clientPresetReqKeys {
		presets = append(presets, preset)
	}
	sort.Strings(presets)

	for _, preset := range presets {
		reqKey := clientPresetReqKeys[preset]
		t.Run(preset, func(t *testing.T) {
			upstreamBody := []byte(`{"code":10000,"message":"ok","data":{"task_id":"parity_` + preset + `"}}`)
			fake := &fakeSubmitClient{resp: &upstream.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: upstreamBody}}
			auditSvc, _, _, _ := newTestAuditService(t, nil, nil, nil)
			h := NewSubmitHandler(fake, auditSvc, nil, nil, slog.New(slog.NewTextHandler(io.Discard, nil))).Routes()

			requestBody := []byte(`{"prompt":"parity test","req_key":"` + reqKey + `"}`)
			req := httptest.NewRequest(http.MethodPost, "/v1/submit", bytes.NewReader(requestBody))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("X-Request-Id", "video-parity-"+preset)
			req = req.WithContext(context.WithValue(req.Context(), sigv4.ContextAPIKeyID, "k-video"))
			rec := httptest.NewRecorder()

			h.ServeHTTP(rec, req)

			if rec.Code != http.StatusOK {
				t.Fatalf("expected status 200, got %d body=%s", rec.Code, rec.Body.String())
			}
			if !bytes.Equal(fake.reqBody, requestBody) {
				t.Fatalf("expected request body passthrough for preset %q", preset)
			}
			if !bytes.Equal(rec.Body.Bytes(), upstreamBody) {
				t.Fatalf("expected response body passthrough for preset %q", preset)
			}
		})
	}
}

func TestClientPresetMatrixParity_DetectsMismatch(t *testing.T) {
	clientPresetReqKeys := mustLoadClientPresetReqKeys(t)
	clientPresetReqKeys["simulated-new-client-preset"] = "jimeng_simulated_not_supported"

	err := detectReqKeyParityMismatch(clientPresetReqKeys, relayAcceptedVideoReqKeys)
	if err == nil {
		t.Fatalf("expected mismatch detection error when client adds unsupported preset")
	}
	if !strings.Contains(err.Error(), "jimeng_simulated_not_supported") {
		t.Fatalf("expected mismatch error to include unsupported req_key, got %v", err)
	}
}

func detectReqKeyParityMismatch(clientPresetReqKeys map[string]string, relayAcceptedReqKeys map[string]struct{}) error {
	missingReqKeys := make([]string, 0)
	for _, reqKey := range clientPresetReqKeys {
		if _, ok := relayAcceptedReqKeys[reqKey]; !ok {
			missingReqKeys = append(missingReqKeys, reqKey)
		}
	}

	if len(missingReqKeys) == 0 {
		return nil
	}

	sort.Strings(missingReqKeys)
	return fmt.Errorf("relay contract missing req_key(s): %s", strings.Join(missingReqKeys, ", "))
}

func mustLoadClientPresetReqKeys(t *testing.T) map[string]string {
	t.Helper()

	clientMatrixPath := filepath.Clean(filepath.Join("..", "..", "..", "..", "client", "internal", "api", "matrix.go"))
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, clientMatrixPath, nil, 0)
	if err != nil {
		t.Fatalf("parse client matrix file %q: %v", clientMatrixPath, err)
	}

	presetConstValues := map[string]string{}
	reqKeyConstValues := map[string]string{}
	presetReqKeyByName := map[string]string{}

	for _, decl := range file.Decls {
		genDecl, ok := decl.(*ast.GenDecl)
		if !ok || genDecl.Tok != token.CONST {
			continue
		}

		for _, spec := range genDecl.Specs {
			valueSpec, ok := spec.(*ast.ValueSpec)
			if !ok {
				continue
			}
			for i, nameIdent := range valueSpec.Names {
				if i >= len(valueSpec.Values) {
					continue
				}
				lit, ok := valueSpec.Values[i].(*ast.BasicLit)
				if !ok || lit.Kind != token.STRING {
					continue
				}
				value := strings.Trim(lit.Value, `"`)
				name := nameIdent.Name
				if strings.HasPrefix(name, "VideoPreset") {
					presetConstValues[name] = value
				}
				if strings.HasPrefix(name, "ReqKey") {
					reqKeyConstValues[name] = value
				}
			}
		}
	}

	for _, decl := range file.Decls {
		funcDecl, ok := decl.(*ast.FuncDecl)
		if !ok || funcDecl.Name == nil || funcDecl.Name.Name != "VideoReqKeyForPreset" || funcDecl.Body == nil {
			continue
		}

		for _, stmt := range funcDecl.Body.List {
			switchStmt, ok := stmt.(*ast.SwitchStmt)
			if !ok {
				continue
			}

			for _, caseClauseNode := range switchStmt.Body.List {
				caseClause, ok := caseClauseNode.(*ast.CaseClause)
				if !ok || len(caseClause.List) == 0 || len(caseClause.Body) == 0 {
					continue
				}
				presetIdent, ok := caseClause.List[0].(*ast.Ident)
				if !ok {
					continue
				}

				returnStmt, ok := caseClause.Body[0].(*ast.ReturnStmt)
				if !ok || len(returnStmt.Results) == 0 {
					continue
				}
				reqKeyIdent, ok := returnStmt.Results[0].(*ast.Ident)
				if !ok {
					continue
				}

				presetReqKeyByName[presetIdent.Name] = reqKeyIdent.Name
			}
		}
	}

	presetReqKeys := map[string]string{}
	for presetConstName, reqKeyConstName := range presetReqKeyByName {
		preset, ok := presetConstValues[presetConstName]
		if !ok {
			t.Fatalf("missing preset constant value for %q in client matrix", presetConstName)
		}
		reqKey, ok := reqKeyConstValues[reqKeyConstName]
		if !ok {
			t.Fatalf("missing req_key constant value for %q in client matrix", reqKeyConstName)
		}
		presetReqKeys[preset] = reqKey
	}

	if len(presetReqKeys) == 0 {
		t.Fatalf("no client preset req_key mapping extracted from %q", clientMatrixPath)
	}

	return presetReqKeys
}
