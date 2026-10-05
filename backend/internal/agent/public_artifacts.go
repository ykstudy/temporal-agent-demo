package agent

import (
	"net/http"
	"net/url"
	"strconv"

	"agentstudio/internal/contracts"
)

func urlQuery(r *http.Request) (url.Values, error) {
	q, e := url.ParseQuery(r.URL.RawQuery)
	if e != nil {
		return nil, fail("invalid_query", "查询参数编码不正确")
	}
	return q, nil
}
func artifactHeaders(w http.ResponseWriter, r Run, body []byte) {
	w.Header().Set("X-Manifest-Hash", r.Control.Audit.Manifest.Hash)
	w.Header().Set("X-Artifact-SHA256", contracts.BytesHash(body))
	w.Header().Set("X-Source-Version", strconv.Itoa(r.SourceVersion))
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
}
func (s *Service) publicArtifactRun(id string) (Run, error) {
	if _, e := s.Store.PublicSnapshot(id); e != nil {
		return Run{}, e
	}
	r, e := s.Store.Get(id)
	if e != nil {
		return r, e
	}
	if r.Control.Audit.Manifest == nil || r.Workspace == "" {
		return r, fail("artifact_not_ready", "尚无已生成的同源工件")
	}
	if r.Control.Audit.Manifest.SourceVersion != int64(r.SourceVersion) || !hashMatches("ArtifactManifest", *r.Control.Audit.Manifest, r.Control.Audit.Manifest.Hash) {
		return r, fail("artifact_source_mismatch", "清单来源或摘要不同")
	}
	return r, nil
}
func (s *Service) v2Artifact(w http.ResponseWriter, req *http.Request) {
	q, e := urlQuery(req)
	if e != nil {
		writeV2Error(w, req, e)
		return
	}
	if len(q) != 1 || len(q["path"]) != 1 || contracts.Validate("agent.schema.json#/$defs/RelativePath", q.Get("path")) != nil {
		writeV2Error(w, req, fail("invalid_path", "需要一个安全的清单相对路径"))
		return
	}
	r, e := s.publicArtifactRun(req.PathValue("id"))
	if e != nil {
		writeV2Error(w, req, e)
		return
	}
	files, e := s.controlToolFiles(r, r.Workspace)
	if e == nil {
		e = controlMatchApproved(r, files)
	}
	if e != nil {
		writeV2Error(w, req, e)
		return
	}
	content, ok := files[q.Get("path")]
	if !ok {
		writeV2Error(w, req, fail("artifact_not_found", "文件不在本来源清单内"))
		return
	}
	body := []byte(content)
	artifactHeaders(w, r, body)
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Write(body)
}
func (s *Service) v2Download(w http.ResponseWriter, req *http.Request) {
	if !noV2Query(w, req) {
		return
	}
	r, e := s.publicArtifactRun(req.PathValue("id"))
	if e != nil {
		writeV2Error(w, req, e)
		return
	}
	for i := len(r.Control.Audit.PackageReceipts) - 1; i >= 0; i-- {
		receipt := r.Control.Audit.PackageReceipts[i]
		if receipt.Status != "succeeded" || receipt.SourceVersion != int64(r.SourceVersion) || receipt.ManifestRef.Hash != r.Control.Audit.Manifest.Hash || receipt.PackageRef == nil {
			continue
		}
		b, e := s.readControlPackage(r, contracts.ObjectReference(*receipt.PackageRef))
		if e != nil {
			writeV2Error(w, req, e)
			return
		}
		artifactHeaders(w, r, b)
		w.Header().Set("Content-Type", "application/zip")
		w.Header().Set("Content-Disposition", "attachment; filename=\""+r.Control.Audit.Plan.ProjectName+".zip\"")
		w.Write(b)
		return
	}
	writeV2Error(w, req, fail("artifact_not_ready", "尚无本来源的已验证包回执"))
}
