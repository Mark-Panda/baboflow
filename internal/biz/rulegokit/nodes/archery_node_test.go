package nodes

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"baboflow/internal/biz/rulegokit/archeryclient"

	"github.com/rulego/rulego"
	"github.com/rulego/rulego/api/types"
	"github.com/rulego/rulego/utils/reflect"
)

// fakeArcheryServer 伪造 Archery 的登录/查询/元数据接口，返回可断言的固定结果。
func fakeArcheryServer(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/login/", func(w http.ResponseWriter, r *http.Request) {
		http.SetCookie(w, &http.Cookie{Name: "csrftoken", Value: "csrf-1", Path: "/"})
		w.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc("/authenticate/", func(w http.ResponseWriter, r *http.Request) {
		http.SetCookie(w, &http.Cookie{Name: "sessionid", Value: "sess-1", Path: "/"})
		json.NewEncoder(w).Encode(map[string]any{"status": 0, "msg": "ok", "data": nil})
	})
	mux.HandleFunc("/query/", func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		sql := r.Form.Get("sql_content")
		if sql == "" {
			json.NewEncoder(w).Encode(map[string]any{"status": 1, "msg": "empty sql"})
			return
		}
		json.NewEncoder(w).Encode(map[string]any{
			"status": 0, "msg": "ok",
			"data": map[string]any{
				"column_list": []string{"cnt"}, "rows": [][]any{{7}},
				"query_time": 0.02,
			},
		})
	})
	mux.HandleFunc("/instance/instance_resource/", func(w http.ResponseWriter, r *http.Request) {
		var items []string
		switch r.URL.Query().Get("resource_type") {
		case "database":
			items = []string{"orders", "users"}
		case "table":
			items = []string{"t1", "t2", "t3"}
		case "column":
			items = []string{"id", "name"}
		}
		json.NewEncoder(w).Encode(map[string]any{"status": 0, "msg": "ok", "data": items})
	})
	return httptest.NewServer(mux)
}

// useFakeFactory 注入一个总是返回指向 fake Archery 的客户端的工厂，并返回清理函数。
func useFakeFactory(t *testing.T, srv *httptest.Server) {
	t.Helper()
	SetArcheryClientFactory(func(_ context.Context, connectionID int64) (*archeryclient.Client, error) {
		if connectionID != 1 {
			return nil, errors.New("unexpected connectionID")
		}
		return archeryclient.New(archeryclient.Config{
			Endpoint: srv.URL, Instance: "prod", Username: "u", Password: "p",
		})
	})
	t.Cleanup(func() { SetArcheryClientFactory(nil) })
}

func TestArcheryQueryNode_FormHasDescCategoryAndFields(t *testing.T) {
	form := reflect.GetComponentForm(NewArcheryQueryNode())
	if form.Desc == "" {
		t.Fatal("expected non-empty Desc")
	}
	if form.Category != "external" {
		t.Fatalf("expected category=external, got %q", form.Category)
	}
	if len(form.Fields) == 0 {
		t.Fatal("expected config fields in form")
	}
}

func TestArcherySchemaNode_FormHasDescCategoryAndFields(t *testing.T) {
	form := reflect.GetComponentForm(NewArcherySchemaNode())
	if form.Desc == "" || form.Category != "external" || len(form.Fields) == 0 {
		t.Fatalf("bad form: desc=%q category=%q fields=%d", form.Desc, form.Category, len(form.Fields))
	}
}

func TestArcheryNodes_Registered(t *testing.T) {
	if _, err := rulego.Registry.NewNode(ArcheryQueryNodeType); err != nil {
		t.Fatalf("archeryQuery should be registered: %v", err)
	}
	if _, err := rulego.Registry.NewNode(ArcherySchemaNodeType); err != nil {
		t.Fatalf("archerySchema should be registered: %v", err)
	}
}

func TestArcheryQueryNode_InitRequiresConnectionID(t *testing.T) {
	n := &ArcheryQueryNode{}
	if err := n.Init(types.Config{}, types.Configuration{}); err == nil {
		t.Fatal("expected error when connectionId missing")
	}
	if err := n.Init(types.Config{}, types.Configuration{"connectionId": 1}); err != nil {
		t.Fatalf("unexpected init error: %v", err)
	}
	if n.config.LimitNum != 100 {
		t.Fatalf("expected default limitNum=100, got %d", n.config.LimitNum)
	}
}

func TestArcherySchemaNode_InitValidatesResource(t *testing.T) {
	n := &ArcherySchemaNode{}
	if err := n.Init(types.Config{}, types.Configuration{"connectionId": 1, "resource": "bogus"}); err == nil {
		t.Fatal("expected error for invalid resource")
	}
	if err := n.Init(types.Config{}, types.Configuration{"connectionId": 1, "resource": "tables"}); err != nil {
		t.Fatalf("unexpected init error: %v", err)
	}
}

func TestArcheryQueryNode_OnMsgUsesMessageSQL(t *testing.T) {
	srv := fakeArcheryServer(t)
	defer srv.Close()
	useFakeFactory(t, srv)

	// 消息是 JSON：{"sql": "..."}，验证 MCP 工具入参能喂给节点。
	dsl := `{
	  "ruleChain": {"id": "chain_archery_q", "root": true},
	  "metadata": {
	    "nodes": [
	      {"id": "q1", "type": "archeryQuery", "name": "查询",
	       "configuration": {"connectionId": 1, "dbName": "orders"}}
	    ],
	    "connections": []
	  }
	}`
	res, err := runDSLForTest("chain_archery_q", []byte(dsl), "JSON", `{"sql":"SELECT count(*) FROM orders"}`, nil)
	if err != nil {
		t.Fatalf("run error: %v", err)
	}
	if res.Err != nil {
		t.Fatalf("run err: %v", res.Err)
	}
	var out map[string]any
	if err := json.Unmarshal([]byte(res.Output), &out); err != nil {
		t.Fatalf("output not JSON: %v (%q)", err, res.Output)
	}
	if out["rowCount"].(float64) != 1 {
		t.Fatalf("expected rowCount=1, got %v", out["rowCount"])
	}
	cols := out["columns"].([]any)
	if len(cols) != 1 || cols[0] != "cnt" {
		t.Fatalf("unexpected columns: %v", cols)
	}
}

func TestArcheryQueryNode_OnMsgFailureWhenNoSQL(t *testing.T) {
	srv := fakeArcheryServer(t)
	defer srv.Close()
	useFakeFactory(t, srv)

	dsl := `{
	  "ruleChain": {"id": "chain_archery_q_fail", "root": true},
	  "metadata": {
	    "nodes": [{"id": "q1", "type": "archeryQuery", "configuration": {"connectionId": 1}}],
	    "connections": []
	  }
	}`
	// 空消息体且无 sql 配置 → 应走 Failure。
	res, err := runDSLForTest("chain_archery_q_fail", []byte(dsl), "JSON", `{}`, nil)
	if err != nil {
		t.Fatalf("run error: %v", err)
	}
	if res.Err == nil {
		t.Fatal("expected failure when no sql provided")
	}
}

func TestArcherySchemaNode_OnMsgListsTables(t *testing.T) {
	srv := fakeArcheryServer(t)
	defer srv.Close()
	useFakeFactory(t, srv)

	dsl := `{
	  "ruleChain": {"id": "chain_archery_s", "root": true},
	  "metadata": {
	    "nodes": [
	      {"id": "s1", "type": "archerySchema", "name": "列表",
	       "configuration": {"connectionId": 1, "resource": "tables", "dbName": "orders"}}
	    ],
	    "connections": []
	  }
	}`
	res, err := runDSLForTest("chain_archery_s", []byte(dsl), "JSON", `{}`, nil)
	if err != nil {
		t.Fatalf("run error: %v", err)
	}
	if res.Err != nil {
		t.Fatalf("run err: %v", res.Err)
	}
	var out map[string]any
	if err := json.Unmarshal([]byte(res.Output), &out); err != nil {
		t.Fatalf("output not JSON: %v (%q)", err, res.Output)
	}
	if out["resource"] != "tables" {
		t.Fatalf("expected resource=tables, got %v", out["resource"])
	}
	if out["count"].(float64) != 3 {
		t.Fatalf("expected count=3, got %v", out["count"])
	}
}

func TestArcherySchemaNode_ColumnsRequiresTable(t *testing.T) {
	srv := fakeArcheryServer(t)
	defer srv.Close()
	useFakeFactory(t, srv)

	dsl := `{
	  "ruleChain": {"id": "chain_archery_s_fail", "root": true},
	  "metadata": {
	    "nodes": [{"id": "s1", "type": "archerySchema",
	       "configuration": {"connectionId": 1, "resource": "columns", "dbName": "orders"}}],
	    "connections": []
	  }
	}`
	res, err := runDSLForTest("chain_archery_s_fail", []byte(dsl), "JSON", `{}`, nil)
	if err != nil {
		t.Fatalf("run error: %v", err)
	}
	if res.Err == nil || !strings.Contains(res.Err.Error(), "tableName") {
		t.Fatalf("expected tableName error, got %v", res.Err)
	}
}
