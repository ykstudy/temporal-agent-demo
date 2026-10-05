package agent

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"strconv"
	"strings"
	"time"

	"agentstudio/internal/contracts"
)

func (s *Store) PublicSnapshot(id string) (json.RawMessage, error) {
	r, e := s.Get(id)
	if e != nil {
		return nil, e
	}
	c := conversationState(r)
	if c == nil || c.LatestSnapshot == nil {
		return nil, fail("legacy_contract_incomplete", "此 Run 缺少从创建开始记录的完整来源，仅可读取 legacy 记录")
	}
	return s.CanonicalObject(id, *c.LatestSnapshot)
}

func readV2Body(req *http.Request) ([]byte, error) {
	typ, _, e := mime.ParseMediaType(req.Header.Get("Content-Type"))
	if e != nil || typ != "application/json" {
		return nil, fail("invalid_content_type", "请求须使用 application/json")
	}
	if req.URL.RawQuery != "" {
		return nil, fail("invalid_query", "此入口不接受查询参数")
	}
	b, e := io.ReadAll(io.LimitReader(req.Body, (64<<10)+1))
	if e != nil {
		return nil, fail("invalid_json", "无法读取完整请求")
	}
	if len(b) > 64<<10 {
		return nil, fail("request_too_large", "请求超过 64 KiB")
	}
	if _, e = contracts.ParseJSON(b); e != nil {
		return nil, fail("invalid_json", "请求必须是无重复键的严格 JSON")
	}
	return b, nil
}
func writeV2Error(w http.ResponseWriter, req *http.Request, err error) {
	var api *APIError
	if errors.As(err, &api) {
		status := errorStatus(api.Code)
		switch api.Code {
		case "stale_target", "object_corrupt", "snapshot_binding_mismatch", "cursor_invalid", "legacy_contract_incomplete", "artifact_source_mismatch", "package_integrity_failed":
			status = 409
		case "object_not_found", "artifact_not_found":
			status = 404
		case "artifact_not_ready":
			status = 409
		}
		value := contracts.ErrorResponse{Error: api.Message, Code: api.Code}
		if api.Code == "legacy_contract_incomplete" {
			url := "/api/runs/" + req.PathValue("id")
			value.LegacyReadURL = &url
		}
		if contracts.ValidateVersion("ErrorResponse", "1.4.0", value) != nil {
			writeError(w, fail("internal_error", "错误结果未满足发布契约"))
			return
		}
		writeJSON(w, status, value)
		return
	}
	writeError(w, err)
}
func noV2Query(w http.ResponseWriter, req *http.Request) bool {
	if req.URL.RawQuery != "" {
		writeV2Error(w, req, fail("invalid_query", "此入口不接受查询参数"))
		return false
	}
	return true
}
func writeV2Receipt(w http.ResponseWriter, receipt contracts.CommandReceipt, created bool) {
	status := 200
	if receipt.Status == "accepted" || receipt.Status == "queued" || receipt.Status == "result_unknown" {
		status = 202
	} else if created && receipt.Status == "processed" {
		status = 201
	}
	writeJSON(w, status, receipt)
}
func (s *Service) registerV2Routes(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/v2/runs", func(w http.ResponseWriter, req *http.Request) {
		b, e := readV2Body(req)
		if e != nil {
			writeV2Error(w, req, e)
			return
		}
		var input contracts.CreateRunRequest
		if contracts.DecodeVersion("CreateRunRequest", "1.4.0", b, &input) != nil {
			writeV2Error(w, req, fail("invalid_request", "创建请求不符合完整 v2 Schema"))
			return
		}
		wire, _, _, e := parseWire("CreateRunRequest", b, input.RunID)
		if e != nil {
			writeV2Error(w, req, e)
			return
		}
		selection := input.DefinitionSelection
		opts := CreateOptions{wire: &wire, ID: input.RunID, Title: input.Title, Content: input.InitialContent, ExecutionMode: input.Modes.ExecutionMode, DecisionMode: input.Modes.DecisionMode, DeliveryMode: input.Modes.DeliveryMode, DefinitionID: selection.DefinitionID, DefinitionVersion: selection.DefinitionVersion, DefinitionHash: selection.DefinitionHash}
		_, e = s.CreateWithOptions(req.Context(), opts)
		receipt, re := s.Store.PublicReceipt(input.RunID, input.CommandID)
		if re == nil && receipt.CommandFingerprint == wire.Fingerprint {
			writeV2Receipt(w, receipt, true)
			return
		}
		if e == nil {
			e = re
		}
		writeV2Error(w, req, e)
	})
	mux.HandleFunc("GET /api/v2/runs", func(w http.ResponseWriter, req *http.Request) {
		if !noV2Query(w, req) {
			return
		}
		out := []json.RawMessage{}
		for _, r := range s.Store.List() {
			if c := conversationState(r); c != nil && c.LatestSnapshot != nil {
				body, e := s.Store.PublicSnapshot(r.ID)
				if e != nil {
					writeV2Error(w, req, e)
					return
				}
				out = append(out, body)
			}
		}
		writeJSON(w, 200, out)
	})
	mux.HandleFunc("GET /api/v2/runs/{id}", func(w http.ResponseWriter, req *http.Request) {
		if !noV2Query(w, req) {
			return
		}
		body, e := s.Store.PublicSnapshot(req.PathValue("id"))
		if e != nil {
			writeV2Error(w, req, e)
			return
		}
		writeJSON(w, 200, body)
	})
	for _, route := range []struct{ path, schema string }{{"messages", "MessageRequest"}, {"actions", "ControlRequest"}} {
		mux.HandleFunc("POST /api/v2/runs/{id}/"+route.path, func(w http.ResponseWriter, req *http.Request) {
			b, e := readV2Body(req)
			if e != nil {
				writeV2Error(w, req, e)
				return
			}
			wire, _, _, e := parseWire(route.schema, b, req.PathValue("id"))
			if e != nil {
				writeV2Error(w, req, e)
				return
			}
			if e = s.bindWire(wire); e != nil {
				writeV2Error(w, req, e)
				return
			}
			if receipt, e := s.Store.PublicReceipt(wire.Command.RunID, wire.Command.ID); e == nil && (receipt.Status == "processed" || receipt.Status == "result_unknown") {
				writeV2Receipt(w, receipt, false)
				return
			}
			_, e = s.Submit(req.Context(), wire.Command)
			receipt, re := s.Store.PublicReceipt(wire.Command.RunID, wire.Command.ID)
			if re == nil {
				writeV2Receipt(w, receipt, false)
				return
			}
			if e == nil {
				e = re
			}
			writeV2Error(w, req, e)
		})
	}
	mux.HandleFunc("GET /api/v2/runs/{id}/commands/{command}", func(w http.ResponseWriter, req *http.Request) {
		if !noV2Query(w, req) {
			return
		}
		receipt, e := s.Store.PublicReceipt(req.PathValue("id"), req.PathValue("command"))
		if e != nil {
			writeV2Error(w, req, e)
			return
		}
		writeJSON(w, 200, receipt)
	})
	mux.HandleFunc("GET /api/v2/runs/{id}/objects/{type}/{object}", s.v2Object)
	mux.HandleFunc("GET /api/v2/runs/{id}/events", s.v2Events)
	mux.HandleFunc("GET /api/v2/runs/{id}/artifacts", s.v2Artifact)
	mux.HandleFunc("GET /api/v2/runs/{id}/download", s.v2Download)
	mux.HandleFunc("/api/v2/", func(w http.ResponseWriter, req *http.Request) {
		writeJSON(w, 404, contracts.ErrorResponse{Code: "not_found", Error: "没有该 v2 入口"})
	})
}

func publicReference(req *http.Request) (contracts.ObjectReference, error) {
	q, e := urlQuery(req)
	if e != nil {
		return contracts.ObjectReference{}, e
	}
	if len(q) != 3 {
		return contracts.ObjectReference{}, fail("invalid_reference", "对象读取需要准确版本、来源和摘要")
	}
	for _, k := range []string{"object_version", "source_version", "hash"} {
		if len(q[k]) != 1 {
			return contracts.ObjectReference{}, fail("invalid_reference", "缺少或重复引用字段")
		}
	}
	version, e1 := strconv.ParseInt(q.Get("object_version"), 10, 64)
	source, e2 := strconv.ParseInt(q.Get("source_version"), 10, 64)
	if e1 != nil || e2 != nil || strconv.FormatInt(version, 10) != q.Get("object_version") || strconv.FormatInt(source, 10) != q.Get("source_version") {
		return contracts.ObjectReference{}, fail("invalid_reference", "版本须为规范正整数")
	}
	ref := contracts.ObjectReference{ObjectType: req.PathValue("type"), ObjectID: req.PathValue("object"), ObjectVersion: version, SourceVersion: source, Hash: q.Get("hash")}
	if contracts.Validate("ObjectReference", ref) != nil {
		return ref, fail("invalid_reference", "引用不符合发布 Schema")
	}
	return ref, nil
}
func (s *Service) v2Object(w http.ResponseWriter, req *http.Request) {
	id := req.PathValue("id")
	if _, e := s.Store.PublicSnapshot(id); e != nil {
		writeV2Error(w, req, e)
		return
	}
	ref, e := publicReference(req)
	if e != nil {
		writeV2Error(w, req, e)
		return
	}
	var b []byte
	switch ref.ObjectType {
	case "package":
		b, e = s.ReadControlPackage(id, ref)
	case "ci_artifact":
		b, e = s.ReadControlCIArtifact(id, ref)
	default:
		b, e = s.Store.CanonicalObject(id, ref)
	}
	if e != nil {
		writeV2Error(w, req, e)
		return
	}
	if ref.ObjectType == "package" || ref.ObjectType == "ci_artifact" {
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Header().Set("X-Artifact-SHA256", contracts.BytesHash(b))
		w.Write(b)
		return
	}
	writeJSON(w, 200, json.RawMessage(b))
}
func snapshotEventID(id string, sequence int) string {
	return "v2_" + contracts.BytesHash([]byte(id))[:20] + "_" + strconv.Itoa(sequence)
}
func (s *Service) v2Events(w http.ResponseWriter, req *http.Request) {
	if !noV2Query(w, req) {
		return
	}
	ch, cancel, e := s.Store.Subscribe(req.PathValue("id"))
	if e != nil {
		writeV2Error(w, req, e)
		return
	}
	defer cancel()
	first := <-ch
	c := conversationState(first)
	if c == nil || c.LatestSnapshot == nil {
		writeV2Error(w, req, fail("legacy_contract_incomplete", "旧 Run 缺少完整快照事件来源"))
		return
	}
	if cursor := req.Header.Get("Last-Event-ID"); cursor != "" {
		prefix := "v2_" + contracts.BytesHash([]byte(first.ID))[:20] + "_"
		v, e := strconv.Atoi(strings.TrimPrefix(cursor, prefix))
		if e != nil || v < 1 || v > first.StateSequence || snapshotEventID(first.ID, v) != cursor {
			writeV2Error(w, req, fail("cursor_invalid", "游标不是此 Run 已提交的序列，请重新读取快照"))
			return
		}
	}
	flush, ok := w.(http.Flusher)
	if !ok {
		writeV2Error(w, req, fail("stream_unsupported", "无法建立状态订阅"))
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Accel-Buffering", "no")
	emit := func(r Run) bool {
		c := conversationState(r)
		if c == nil || c.LatestSnapshot == nil {
			return false
		}
		event := contracts.SnapshotEvent{ContractVersion: "1.4.0", SchemaVersion: 2, EventID: snapshotEventID(r.ID, r.StateSequence), RunID: r.ID, SourceVersion: int64(r.SourceVersion), StateSequence: int64(r.StateSequence), RunRef: contracts.SnapshotEventRunRef(*c.LatestSnapshot), OccurredAt: r.UpdatedAt}
		if contracts.Validate("SnapshotEvent", event) != nil {
			return false
		}
		body, _ := json.Marshal(event)
		if _, e := fmt.Fprintf(w, "event: snapshot\nid: %s\ndata: %s\n\n", event.EventID, body); e != nil {
			return false
		}
		flush.Flush()
		return true
	}
	if !emit(first) {
		return
	}
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-req.Context().Done():
			return
		case run := <-ch:
			if !emit(run) {
				return
			}
		case <-ticker.C:
			if _, e = fmt.Fprint(w, ": keepalive\n\n"); e != nil {
				return
			}
			flush.Flush()
		}
	}
}
