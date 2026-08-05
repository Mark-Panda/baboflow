package archeryclient

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

// fakeArchery 起一个在内存中伪造 Archery 的服务器：
// /login/ 种 csrftoken；/authenticate/ 校验口令并种 sessionid；
// /query/ 返回一行结果；/instance/instance_resource/ 返回清单。
// loginCount 记录 /authenticate/ 被调用的次数（用于断言自动重登）。
func fakeArchery(t *testing.T, loginCount *int32) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/login/", func(w http.ResponseWriter, r *http.Request) {
		http.SetCookie(w, &http.Cookie{Name: "csrftoken", Value: "csrf-123", Path: "/"})
		w.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc("/authenticate/", func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(loginCount, 1)
		if err := r.ParseForm(); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		if r.Form.Get("username") != "alice" || r.Form.Get("password") != "secret" {
			json.NewEncoder(w).Encode(map[string]any{"status": 1, "msg": "用户名或密码错误"})
			return
		}
		http.SetCookie(w, &http.Cookie{Name: "sessionid", Value: "sess-abc", Path: "/"})
		json.NewEncoder(w).Encode(map[string]any{"status": 0, "msg": "ok", "data": nil})
	})
	mux.HandleFunc("/query/", func(w http.ResponseWriter, r *http.Request) {
		if c, err := r.Cookie("sessionid"); err != nil || c.Value != "sess-abc" {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		if err := r.ParseForm(); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		if r.Form.Get("sql_content") == "BAD" {
			json.NewEncoder(w).Encode(map[string]any{"status": 1, "msg": "syntax error"})
			return
		}
		json.NewEncoder(w).Encode(map[string]any{
			"status": 0, "msg": "ok",
			"data": map[string]any{
				"column_list": []string{"n"}, "rows": [][]any{{42}},
				"query_time": 0.01, "affected_rows": 0,
			},
		})
	})
	mux.HandleFunc("/instance/instance_resource/", func(w http.ResponseWriter, r *http.Request) {
		if c, err := r.Cookie("sessionid"); err != nil || c.Value != "sess-abc" {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		rt := r.URL.Query().Get("resource_type")
		var items []string
		switch rt {
		case "database":
			items = []string{"orders", "users"}
		case "table":
			items = []string{"orders", "order_items"}
		default:
			items = []string{}
		}
		json.NewEncoder(w).Encode(map[string]any{"status": 0, "msg": "ok", "data": items})
	})
	return httptest.NewServer(mux)
}

func testConfig(endpoint string) Config {
	return Config{Endpoint: endpoint, Instance: "prod", Username: "alice", Password: "secret"}
}

func TestClient_QueryAutoLogin(t *testing.T) {
	var logins int32
	srv := fakeArchery(t, &logins)
	defer srv.Close()

	cli, err := New(testConfig(srv.URL))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	res, err := cli.Query("orders", "public", "SELECT 1", 100)
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if len(res.ColumnList) != 1 || res.ColumnList[0] != "n" {
		t.Fatalf("unexpected columns: %+v", res.ColumnList)
	}
	if len(res.Rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(res.Rows))
	}
	if atomic.LoadInt32(&logins) != 1 {
		t.Fatalf("expected exactly 1 auto-login, got %d", logins)
	}
}

func TestClient_ServerErrorOnBadSQL(t *testing.T) {
	var logins int32
	srv := fakeArchery(t, &logins)
	defer srv.Close()

	cli, _ := New(testConfig(srv.URL))
	_, err := cli.Query("orders", "public", "BAD", 100)
	if err == nil || !strings.Contains(err.Error(), "syntax error") {
		t.Fatalf("expected syntax error, got %v", err)
	}
	var se *ServerError
	if !errors.As(err, &se) {
		t.Fatalf("expected *ServerError, got %T", err)
	}
}

func TestClient_QueryErrorIncludesArcheryAPIContext(t *testing.T) {
	var logins int32
	srv := fakeArchery(t, &logins)
	defer srv.Close()

	cli, _ := New(testConfig(srv.URL))
	_, err := cli.Query("orders", "public", "BAD", 100)
	if err == nil || !strings.Contains(err.Error(), "Archery /query/") {
		t.Fatalf("expected Archery query API context, got %v", err)
	}
}

func TestClient_Resource(t *testing.T) {
	var logins int32
	srv := fakeArchery(t, &logins)
	defer srv.Close()

	cli, _ := New(testConfig(srv.URL))
	dbs, err := cli.Resource(ResDatabase, "", "", "")
	if err != nil {
		t.Fatalf("Resource database: %v", err)
	}
	if len(dbs) != 2 || dbs[0] != "orders" {
		t.Fatalf("unexpected databases: %v", dbs)
	}
	tables, err := cli.Resource(ResTable, "orders", "public", "")
	if err != nil {
		t.Fatalf("Resource table: %v", err)
	}
	if len(tables) != 2 {
		t.Fatalf("unexpected tables: %v", tables)
	}
}

func TestClient_LoginBadPassword(t *testing.T) {
	var logins int32
	srv := fakeArchery(t, &logins)
	defer srv.Close()

	cfg := testConfig(srv.URL)
	cfg.Password = "wrong"
	cli, _ := New(cfg)
	if err := cli.Login(); err != ErrAuthFailed {
		t.Fatalf("expected ErrAuthFailed, got %v", err)
	}
}

func TestClient_RejectsEmptyEndpoint(t *testing.T) {
	if _, err := New(Config{Endpoint: "  "}); err == nil {
		t.Fatal("expected error for empty endpoint")
	}
}

func TestClient_DefaultSchemaForPostgreSQL(t *testing.T) {
	postgres, err := New(Config{Endpoint: "http://archery.test", DBType: "pgsql"})
	if err != nil {
		t.Fatalf("New postgres client: %v", err)
	}
	if got := postgres.DefaultSchema(""); got != "public" {
		t.Fatalf("postgres default schema = %q, want public", got)
	}
	if got := postgres.DefaultSchema("custom"); got != "custom" {
		t.Fatalf("explicit postgres schema = %q, want custom", got)
	}

	mysql, err := New(Config{Endpoint: "http://archery.test", DBType: "mysql"})
	if err != nil {
		t.Fatalf("New mysql client: %v", err)
	}
	if got := mysql.DefaultSchema(""); got != "" {
		t.Fatalf("mysql default schema = %q, want empty", got)
	}
}
