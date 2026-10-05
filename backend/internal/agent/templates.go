package agent

import (
	"bytes"
	_ "embed"
	"fmt"
	"path"
	"sort"
	"strconv"
	"strings"
	"text/template"
	"time"
)

func hasSkill(p Plan, id string) bool {
	for _, s := range p.Skills {
		if s == id {
			return true
		}
	}
	return false
}
func normalizeLogging(p Plan) Plan {
	if p.LogOutput == "" {
		p.LogOutput = "console"
	}
	if p.LogPath == "" {
		p.LogPath = "logs/app.log"
	}
	if p.LogMaxSize == 0 {
		p.LogMaxSize = 10 << 20
	}
	if p.LogMaxBackups == 0 {
		p.LogMaxBackups = 3
	}
	if p.LogMaxAge == "" {
		p.LogMaxAge = "168h"
	}
	if p.LogOnWriteError == "" {
		p.LogOnWriteError = "degrade"
	}
	return p
}
func ValidatePlan(p Plan) error {
	p = normalizeLogging(p)
	if !projectName.MatchString(p.ProjectName) {
		return fail("invalid_project_name", "项目名须为 2–63 位小写字母、数字或连字符，并以字母开头")
	}
	if p.TechStack != "golang" {
		return fail("unsupported_stack", "当前仅支持 Golang")
	}
	seen := map[string]bool{}
	for _, s := range p.Skills {
		if s != "add_database" && s != "add_auth" && s != "add_logging" {
			return fail("invalid_skill", "unknown skill: "+s)
		}
		if seen[s] {
			return fail("invalid_skill", "duplicate skill: "+s)
		}
		seen[s] = true
	}
	if p.RepoProvider != "local" && p.RepoProvider != "gitlab" {
		return fail("invalid_provider", "repo_provider must be local or gitlab")
	}
	if p.RepoVisibility != "private" {
		return fail("invalid_visibility", "本版本仅允许 private 仓库")
	}
	if hasSkill(p, "add_database") {
		if p.Database != "postgres" && p.Database != "mysql" && p.Database != "sqlite" {
			return fail("invalid_database", "unsupported database")
		}
		if p.ORM != "gorm" && p.ORM != "sqlx" {
			return fail("invalid_orm", "unsupported ORM")
		}
	}
	if hasSkill(p, "add_auth") {
		if p.Auth != "jwt" && p.Auth != "session" {
			return fail("invalid_auth", "unsupported auth method")
		}
		d, e := time.ParseDuration(p.TokenTTL)
		if e != nil || d <= 0 || d > 30*24*time.Hour {
			return fail("invalid_ttl", "token_ttl must be a Go duration between 1ns and 720h")
		}
	}
	if hasSkill(p, "add_logging") {
		if p.LogLevel != "debug" && p.LogLevel != "info" && p.LogLevel != "warn" && p.LogLevel != "error" {
			return fail("invalid_log_level", "unsupported log level")
		}
		if p.LogFormat != "json" && p.LogFormat != "text" {
			return fail("invalid_log_format", "unsupported log format")
		}
		if p.LogOutput != "console" && p.LogOutput != "file" {
			return fail("invalid_log_output", "log_output must be console or file")
		}
		if !strings.HasPrefix(p.LogPath, "logs/") || p.LogPath == "." || path.IsAbs(p.LogPath) || path.Clean(p.LogPath) != p.LogPath || strings.HasPrefix(p.LogPath, "../") || strings.ContainsAny(p.LogPath, "\\\"\n\r\t") || len(p.LogPath) > 160 {
			return fail("invalid_log_path", "log_path must be a controlled path under logs/")
		}
		for _, part := range strings.Split(p.LogPath, "/") {
			if part == ".." || part == "." || part == "" {
				return fail("invalid_log_path", "log path has invalid components")
			}
		}
		if p.LogMaxSize < 1<<20 || p.LogMaxSize > 1<<30 || p.LogMaxBackups < 1 || p.LogMaxBackups > 100 {
			return fail("invalid_log_rotation", "rotation size/count outside bounded range")
		}
		age, err := time.ParseDuration(p.LogMaxAge)
		if err != nil || age < time.Hour || age > 365*24*time.Hour {
			return fail("invalid_log_rotation", "log_max_age must be between 1h and 8760h")
		}
		if p.LogOnWriteError != "degrade" && p.LogOnWriteError != "fail_closed" {
			return fail("invalid_log_policy", "log_on_write_error must be degrade or fail_closed")
		}

	}
	return nil
}

// RenderFiles is shared by previews and execution: confirming a preview executes
// precisely the displayed templates, rather than a second model generation.
func RenderFiles(p Plan) (map[string]string, error) {
	p = normalizeLogging(p)
	if e := ValidatePlan(p); e != nil {
		return nil, e
	}
	files := map[string]string{}
	render := func(path, source string) error {
		t, e := template.New(path).Funcs(template.FuncMap{"quote": strconv.Quote, "skill": func(id string) bool { return hasSkill(p, id) }}).Parse(source)
		if e != nil {
			return e
		}
		var b bytes.Buffer
		if e = t.Execute(&b, p); e != nil {
			return e
		}
		files[path] = b.String()
		return nil
	}
	base := baseTemplateSources()
	for path, source := range base {
		if e := render(path, source); e != nil {
			return nil, e
		}
	}
	if hasSkill(p, "add_database") {
		var src string
		var deps string
		if p.ORM == "gorm" {
			driver := p.Database
			if driver == "postgres" {
				deps = "gorm.io/driver/postgres v1.6.0"
			} else if driver == "mysql" {
				deps = "gorm.io/driver/mysql v1.6.0"
			} else {
				deps = "gorm.io/driver/sqlite v1.6.0"
			}
			deps += "\n    gorm.io/gorm v1.31.0"
			src = fmt.Sprintf(`package database

import (
    "errors"
    "gorm.io/gorm"
    "gorm.io/driver/%s"
)
func Open(dsn string) (*gorm.DB, error) {
    if dsn == "" { return nil, errors.New("DATABASE_DSN is required") }
    return gorm.Open(%s.Open(dsn), &gorm.Config{})
}
func Check(dsn string) error {
    db,err:=Open(dsn);if err!=nil{return err};connection,err:=db.DB();if err!=nil{return err};defer connection.Close();return connection.Ping()
}
`, driver, driver)
		} else {
			driver, imp := p.Database, "github.com/lib/pq"
			deps = "github.com/jmoiron/sqlx v1.4.0\n    github.com/lib/pq v1.10.9"
			if driver == "postgres" {
				driver = "postgres"
			}
			if p.Database == "mysql" {
				driver = "mysql"
				imp = "github.com/go-sql-driver/mysql"
				deps = "github.com/jmoiron/sqlx v1.4.0\n    github.com/go-sql-driver/mysql v1.9.3"
			}
			if p.Database == "sqlite" {
				driver = "sqlite3"
				imp = "github.com/mattn/go-sqlite3"
				deps = "github.com/jmoiron/sqlx v1.4.0\n    github.com/mattn/go-sqlite3 v1.14.32"
			}
			src = fmt.Sprintf(`package database

import (
    "errors"
    "github.com/jmoiron/sqlx"
    _ "%s"
)
func Open(dsn string) (*sqlx.DB, error) {
    if dsn == "" { return nil, errors.New("DATABASE_DSN is required") }
    return sqlx.Connect("%s", dsn)
}
func Check(dsn string) error {
    db,err:=Open(dsn);if err!=nil{return err};defer db.Close();return db.Ping()
}
`, imp, driver)
		}
		files["go.mod"] += "\nrequire (\n    " + deps + "\n)\n"
		databaseType := "*gorm.DB"
		execMigration := "return db.Exec(initialMigration).Error"
		if p.ORM == "sqlx" {
			databaseType = "*sqlx.DB"
			execMigration = "_,err:=db.Exec(initialMigration);return err"
		}
		src += fmt.Sprintf("\n// Migrate is an explicit application call; server startup never applies migrations.\nconst initialMigration = %q\nfunc Migrate(db %s) error { if db==nil {return errors.New(\"database connection is required\")};%s }\n", "CREATE TABLE IF NOT EXISTS schema_versions (version INTEGER PRIMARY KEY);", databaseType, execMigration)
		files["internal/database/database.go"] = src
		files["internal/database/database_test.go"] = `package database
import "testing"
func TestMissingDSN(t *testing.T) { if _,err:=Open("");err==nil {t.Fatal("missing database DSN must fail")} }
`
		if p.Database == "sqlite" {
			dbTest := `package database
import("path/filepath";"testing")
func TestIsolatedSQLiteMigration(t *testing.T){
    db,err:=Open(filepath.Join(t.TempDir(),"isolated.db"));if err!=nil{t.Fatal(err)}
    CLOSE_DB
    if err=Migrate(db);err!=nil{t.Fatal(err)};if err=Migrate(db);err!=nil{t.Fatal(err)}
    CHECK_TABLE
}
`
			if p.ORM == "gorm" {
				dbTest = strings.ReplaceAll(dbTest, "CLOSE_DB", "connection,err:=db.DB();if err!=nil{t.Fatal(err)};defer connection.Close()")
				dbTest = strings.ReplaceAll(dbTest, "CHECK_TABLE", `if !db.Migrator().HasTable("schema_versions"){t.Fatal("migration did not create table")}`)
			} else {
				dbTest = strings.ReplaceAll(dbTest, "CLOSE_DB", "defer db.Close()")
				dbTest = strings.ReplaceAll(dbTest, "CHECK_TABLE", `var count int;if err=db.Get(&count,"SELECT count(*) FROM schema_versions");err!=nil{t.Fatal(err)}`)
			}
			files["internal/database/isolation_test.go"] = dbTest
		}
		files["migrations/001_initial.sql"] = "-- Review this example migration before applying it.\nCREATE TABLE IF NOT EXISTS schema_versions (version INTEGER PRIMARY KEY);\n"
		if p.Database == "sqlite" {
			files["Dockerfile"] = strings.ReplaceAll(files["Dockerfile"], "CGO_ENABLED=0", "CGO_ENABLED=1")
			files["Dockerfile"] = strings.ReplaceAll(files["Dockerfile"], "distroless/static-debian12", "distroless/base-debian12")
		}
	}
	if hasSkill(p, "add_auth") {
		if p.Auth == "jwt" {
			files["internal/auth/auth.go"] = strings.ReplaceAll(jwtSource, "TTL_VALUE", p.TokenTTL)
			files["internal/auth/auth_test.go"] = jwtTests
		} else {
			files["internal/auth/auth.go"] = strings.ReplaceAll(sessionSource, "TTL_VALUE", p.TokenTTL)
			files["internal/auth/auth_test.go"] = sessionTests
		}
	}
	if hasSkill(p, "add_logging") {
		if e := render("internal/logging/logging.go", loggingSource); e != nil {
			return nil, e
		}
		if e := render("internal/logging/logging_test.go", loggingTests); e != nil {
			return nil, e
		}

	}
	if hasSkill(p, "add_logging") && p.LogOutput == "file" {
		if p.LogPath == "AGENT-MANIFEST.json" {
			return nil, fail("invalid_log_path", "log path overlaps delivery manifest")
		}
		for name := range files {
			if p.LogPath == name || strings.HasPrefix(name, p.LogPath+"/") || strings.HasPrefix(p.LogPath, name+"/") {
				return nil, fail("invalid_log_path", "log path overlaps source files")
			}
			for i := 1; i <= p.LogMaxBackups; i++ {
				if p.LogPath+"."+strconv.Itoa(i) == name {
					return nil, fail("invalid_log_path", "log backups overlap source files")
				}
			}
		}
	}
	return files, nil
}
func changesFor(files map[string]string) []Change {
	paths := make([]string, 0, len(files))
	for p := range files {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	out := make([]Change, 0, len(paths))
	for _, p := range paths {
		out = append(out, Change{Path: p, Before: "", After: files[p]})
	}
	return out
}

const jwtSource = `package auth

import (
    "crypto/hmac"
    "crypto/sha256"
    "encoding/base64"
    "encoding/json"
    "errors"
    "net/http"
    "strings"
    "time"
)
type Claims struct { Subject string ` + "`json:\"sub\"`" + `; Expires int64 ` + "`json:\"exp\"`" + ` }
func Issue(subject, secret string) (string,error) {
    if subject=="" || len(secret)<32 {return "",errors.New("subject and a secret of at least 32 characters are required")}
    ttl,_:=time.ParseDuration("TTL_VALUE")
    payload,_:=json.Marshal(Claims{Subject:subject,Expires:time.Now().Add(ttl).Unix()})
    header:=base64.RawURLEncoding.EncodeToString([]byte(` + "`{\"alg\":\"HS256\",\"typ\":\"JWT\"}`" + `))
    unsigned:=header+"."+base64.RawURLEncoding.EncodeToString(payload)
    mac:=hmac.New(sha256.New,[]byte(secret));mac.Write([]byte(unsigned))
    return unsigned+"."+base64.RawURLEncoding.EncodeToString(mac.Sum(nil)),nil
}
func Verify(token,secret string) (Claims,error) {
    var c Claims
    if len(secret)<32 {return c,errors.New("invalid authentication configuration")}
    parts:=strings.Split(token,".");if len(parts)!=3{return c,errors.New("invalid token")}
    header,err:=base64.RawURLEncoding.DecodeString(parts[0]);if err!=nil{return c,errors.New("invalid header")}
    var h map[string]string;if json.Unmarshal(header,&h)!=nil||h["alg"]!="HS256" {return c,errors.New("unsupported token algorithm")}
    sig,err:=base64.RawURLEncoding.DecodeString(parts[2]);if err!=nil{return c,errors.New("invalid signature")}
    mac:=hmac.New(sha256.New,[]byte(secret));mac.Write([]byte(parts[0]+"."+parts[1]))
    if !hmac.Equal(sig,mac.Sum(nil)){return c,errors.New("invalid signature")}
    payload,err:=base64.RawURLEncoding.DecodeString(parts[1]);if err!=nil||json.Unmarshal(payload,&c)!=nil||c.Subject==""||c.Expires<=time.Now().Unix(){return Claims{},errors.New("invalid or expired claims")}
    return c,nil
}
func Middleware(secret string,next http.Handler) http.Handler {return http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){
    header:=r.Header.Get("Authorization");if !strings.HasPrefix(header,"Bearer "){http.Error(w,"unauthorized",401);return}
    if _,err:=Verify(strings.TrimPrefix(header,"Bearer "),secret);err!=nil{http.Error(w,"unauthorized",401);return};next.ServeHTTP(w,r)
})}
`
const jwtTests = `package auth
import("crypto/hmac";"crypto/sha256";"encoding/base64";"encoding/json";"net/http";"net/http/httptest";"strings";"testing";"time")
func TestSignedToken(t *testing.T) {secret:=strings.Repeat("a",32);token,err:=Issue("user",secret);if err!=nil{t.Fatal(err)};c,err:=Verify(token,secret);if err!=nil||c.Subject!="user"{t.Fatal("valid token rejected")};if _,err=Verify(token,strings.Repeat("b",32));err==nil{t.Fatal("wrong secret accepted")};if _,err=Verify(token+"x",secret);err==nil{t.Fatal("tampered token accepted")}}
func TestClaimsAndAlgorithm(t *testing.T){
    secret:=strings.Repeat("z",32)
    makeToken:=func(header string,c Claims)string{payload,_:=json.Marshal(c);unsigned:=base64.RawURLEncoding.EncodeToString([]byte(header))+"."+base64.RawURLEncoding.EncodeToString(payload);mac:=hmac.New(sha256.New,[]byte(secret));mac.Write([]byte(unsigned));return unsigned+"."+base64.RawURLEncoding.EncodeToString(mac.Sum(nil))}
    for _,candidate:=range []struct{header string;claims Claims}{{"{\"alg\":\"none\"}",Claims{Subject:"user",Expires:time.Now().Add(time.Hour).Unix()}},{"{\"alg\":\"HS256\"}",Claims{Subject:"user",Expires:1}},{"{\"alg\":\"HS256\"}",Claims{Expires:time.Now().Add(time.Hour).Unix()}}}{if _,err:=Verify(makeToken(candidate.header,candidate.claims),secret);err==nil{t.Fatal("invalid claims or algorithm accepted")}}
}
func TestMissingCredentials(t *testing.T){rr:=httptest.NewRecorder();Middleware("",http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){t.Fatal("unauthorized handler reached")})).ServeHTTP(rr,httptest.NewRequest("GET","/",nil));if rr.Code!=401{t.Fatal("missing credentials accepted")}}
`
const sessionSource = `package auth
import("crypto/hmac";"crypto/rand";"crypto/sha256";"encoding/hex";"errors";"net/http";"strings";"sync";"time")
type Session struct {Subject string;Expires time.Time}
var sessions=struct{sync.Mutex;values map[string]Session}{values:map[string]Session{}}
func Issue(subject,secret string)(string,error){if subject==""||len(secret)<32{return "",errors.New("subject and a secret of at least 32 characters are required")};var b [32]byte;if _,err:=rand.Read(b[:]);err!=nil{return "",err};id:=hex.EncodeToString(b[:]);mac:=hmac.New(sha256.New,[]byte(secret));mac.Write([]byte(id));ttl,_:=time.ParseDuration("TTL_VALUE");sessions.Lock();sessions.values[id]=Session{subject,time.Now().Add(ttl)};sessions.Unlock();return id+"."+hex.EncodeToString(mac.Sum(nil)),nil}
func Verify(token,secret string)(Session,error){if len(secret)<32{return Session{},errors.New("invalid authentication configuration")};parts:=strings.Split(token,".");if len(parts)!=2{return Session{},errors.New("invalid session")};sig,err:=hex.DecodeString(parts[1]);if err!=nil{return Session{},err};mac:=hmac.New(sha256.New,[]byte(secret));mac.Write([]byte(parts[0]));if !hmac.Equal(sig,mac.Sum(nil)){return Session{},errors.New("invalid signature")};sessions.Lock();defer sessions.Unlock();s,ok:=sessions.values[parts[0]];if !ok||time.Now().After(s.Expires){delete(sessions.values,parts[0]);return Session{},errors.New("session expired")};return s,nil}
func Middleware(secret string,next http.Handler)http.Handler{return http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){c,err:=r.Cookie("session");if err!=nil{http.Error(w,"unauthorized",401);return};if _,err=Verify(c.Value,secret);err!=nil{http.Error(w,"unauthorized",401);return};next.ServeHTTP(w,r)})}
func SetCookie(w http.ResponseWriter,token string){ttl,_:=time.ParseDuration("TTL_VALUE");http.SetCookie(w,&http.Cookie{Name:"session",Value:token,Path:"/",HttpOnly:true,Secure:true,SameSite:http.SameSiteLaxMode,MaxAge:int(ttl.Seconds())})}
func Revoke(token string){id:=strings.Split(token,".")[0];sessions.Lock();delete(sessions.values,id);sessions.Unlock()}
`
const sessionTests = `package auth
import("net/http";"net/http/httptest";"strings";"testing";"time")
func TestSession(t *testing.T){secret:=strings.Repeat("a",32);token,err:=Issue("user",secret);if err!=nil{t.Fatal(err)};s,err:=Verify(token,secret);if err!=nil||s.Subject!="user"{t.Fatal("valid session rejected")};if _,err=Verify(token,strings.Repeat("b",32));err==nil{t.Fatal("wrong key accepted")};Revoke(token);if _,err=Verify(token,secret);err==nil{t.Fatal("revoked session accepted")}}
func TestSessionExpiryAndCookie(t *testing.T){
    secret:=strings.Repeat("a",32);token,err:=Issue("user",secret);if err!=nil{t.Fatal(err)}
    id:=strings.Split(token,".")[0];sessions.Lock();sessions.values[id]=Session{Subject:"user",Expires:time.Now().Add(-time.Second)};sessions.Unlock()
    if _,err=Verify(token,secret);err==nil{t.Fatal("expired session accepted")}
    rr:=httptest.NewRecorder();SetCookie(rr,token);cookies:=rr.Result().Cookies();if len(cookies)!=1||!cookies[0].HttpOnly||!cookies[0].Secure||cookies[0].SameSite!=http.SameSiteLaxMode{t.Fatal("unsafe session cookie")}
}
`

//go:embed templates/logging.go.tmpl
var loggingSource string

//go:embed templates/logging_test.go.tmpl
var loggingTests string

// baseTemplateSources is used only when publishing a new template release.
func baseTemplateSources() map[string]string {
	return map[string]string{
		"go.mod": `module {{.ProjectName}}

go 1.25.0
`,
		"cmd/server/main.go": `package main

import (
    "log"
    "net/http"
    "os"
    "{{.ProjectName}}/internal/server"
{{if skill "add_logging"}}    "{{.ProjectName}}/internal/logging"
{{end}}{{if skill "add_database"}}    "{{.ProjectName}}/internal/database"
{{end}})

func main() {
{{if skill "add_logging"}}    if err := logging.Configure(os.Stdout); err != nil { log.Fatal("logging initialization failed") }
    defer logging.Close()
{{end}}{{if skill "add_database"}}    if dsn := os.Getenv("DATABASE_DSN"); dsn != "" {
        if err := database.Check(dsn); err != nil { log.Fatal("database connection failed; check DATABASE_DSN") }
    }
{{end}}    addr := os.Getenv("HTTP_ADDR")
    if addr == "" { addr = ":8080" }
    log.Printf("listening on %s", addr)
    if err := http.ListenAndServe(addr, server.New()); err != nil { log.Fatal(err) }
}
`,
		"internal/server/server.go": `package server

import (
    "encoding/json"
    "net/http"
{{if skill "add_logging"}}    "{{.ProjectName}}/internal/logging"
{{end}}{{if skill "add_auth"}}    "os"
    "{{.ProjectName}}/internal/auth"
{{end}})

func New() http.Handler {
    mux := http.NewServeMux()
    mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
        w.Header().Set("Content-Type", "application/json")
{{if skill "add_logging"}}        health := logging.Status()
        if !health.Ready { w.WriteHeader(http.StatusServiceUnavailable) }
        _ = json.NewEncoder(w).Encode(map[string]any{"status":health.Status, "project":"{{.ProjectName}}", "logging":health{{if skill "add_database"}}, "database":"connection not checked by health endpoint"{{end}}})
{{else}}        _ = json.NewEncoder(w).Encode(map[string]string{"status":"ok", "project":"{{.ProjectName}}"{{if skill "add_database"}}, "database":"connection not checked by health endpoint"{{end}}})
{{end}}    })
{{if skill "add_logging"}}    mux.HandleFunc("GET /readyz", func(w http.ResponseWriter, r *http.Request) {
        if !logging.Ready() { http.Error(w,"logging unavailable",http.StatusServiceUnavailable);return }
        w.WriteHeader(http.StatusOK)
    })
{{end}}
{{if skill "add_auth"}}    mux.Handle("GET /api/private", auth.Middleware(os.Getenv("AUTH_SECRET"), http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("authenticated")) })))
{{end}}{{if skill "add_logging"}}    return http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){
        if r.Method!="GET" && r.Method!="HEAD" && r.Method!="OPTIONS" && !logging.Ready() { http.Error(w,"service not ready",http.StatusServiceUnavailable);return }
        mux.ServeHTTP(w,r)
    })
{{else}}    return mux
{{end}}}
`,
		"internal/server/server_test.go": `package server

import (
    "net/http/httptest"
    "testing"
    "encoding/json"
{{if skill "add_logging"}}    "io"
    "{{.ProjectName}}/internal/logging"
{{end}})
func TestHealth(t *testing.T) {
{{if skill "add_logging"}}    t.Chdir(t.TempDir())
    if err:=logging.Configure(io.Discard);err!=nil{t.Fatal(err)};defer logging.Close()
{{end}}    rr := httptest.NewRecorder()
    New().ServeHTTP(rr, httptest.NewRequest("GET", "/healthz", nil))
    if rr.Code != 200 { t.Fatalf("health: %d", rr.Code) }
    var body map[string]any
    if err:=json.Unmarshal(rr.Body.Bytes(), &body); err!=nil || body["status"]!="ok" { t.Fatal("invalid health response") }
}
`,
		".gitignore": `.env
.env.*
!.env.example
*.pem
*.key
bin/
coverage.out
logs/
`,
		".env.example": `HTTP_ADDR=:8080
{{if skill "add_database"}}DATABASE_DSN=
{{end}}{{if skill "add_auth"}}AUTH_SECRET=
{{end}}`,
		".gitlab-ci.yml": `image: golang:1.25.0-bookworm
stages: [test, build]
test:
  stage: test
  script:
    - go mod download
    - go test ./... -race -coverprofile=coverage.out
  artifacts:
    paths: [coverage.out]
build:
  stage: build
  script:
    - mkdir -p bin
    - go build -o bin/server ./cmd/server
  artifacts:
    paths: [bin/server]
`,
		"Dockerfile": `FROM golang:1.25.0-bookworm AS build
WORKDIR /app
COPY . .
RUN CGO_ENABLED=0 go build -o /server ./cmd/server
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /server /server
EXPOSE 8080
ENTRYPOINT ["/server"]
`,
		"README.md": `# {{.ProjectName}}

{{.Description}}

## Run

    go mod tidy
    go test ./...
    go build ./...
    go run ./cmd/server

Health: http://localhost:8080/healthz

Secrets belong in environment variables. Never commit .env, PEM or key files.

{{if skill "add_database"}}## Database

Generated {{.ORM}} adapter for {{.Database}}. Set DATABASE_DSN to check the configured connection at server startup, or call database.Open from your application layer. Without DATABASE_DSN, the health server starts without a database connection. /healthz does not assert database readiness.
Call database.Migrate only against a separately approved database target; the example is a schema_versions table and is not run at server startup. The Agent runs SQLite connection/migration tests in temporary databases; it never connects to your database. PostgreSQL/MySQL connection and migration require your isolated test environment.
{{end}}{{if skill "add_auth"}}## Authentication

Generated {{.Auth}} middleware. Set AUTH_SECRET to at least 32 characters. The private endpoint rejects requests when credentials are missing. Add your user identity validation before issuing tokens; no default username or password is provided.
Token lifetime: {{.TokenTTL}}. JWT revocation is not provided by this stateless skeleton; rotate the key or add a deny list. Session storage is process memory and all sessions expire on restart; it is not shared between instances.
{{end}}{{if skill "add_logging"}}## Logging

Structured slog logger: {{.LogLevel}} / {{.LogFormat}} / {{.LogOutput}}.
File path: {{.LogPath}} relative to the project working directory; max_size={{.LogMaxSize}} bytes, max_backups={{.LogMaxBackups}}, max_age={{.LogMaxAge}}.
Initialization failure stops startup; runtime errors appear in /healthz with a failure count. Policy={{.LogOnWriteError}}; fail_closed also returns 503 for readiness and write requests. Sensitive attributes and configured environment secrets are redacted. Run a single writer per configured log path; file output is not a multi-process collector.
{{end}}`,
	}
}
