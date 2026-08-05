package nodes

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"baboflow/internal/biz/rulegokit"
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
	mux.HandleFunc("/group/user_all_instances/", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"status": 0, "msg": "ok", "data": []map[string]any{
			{"id": 1, "instance_name": "prod", "db_type": "mysql"},
		}})
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
	SetArcheryClientFactory(func(_ context.Context, instanceID int64) (*archeryclient.Client, error) {
		if instanceID != 1 {
			return nil, errors.New("unexpected instanceID")
		}
		return archeryclient.New(archeryclient.Config{
			Endpoint: srv.URL, Instance: "prod", Username: "u", Password: "p",
		})
	})
	SetArcheryInstanceListFactory(func(_ context.Context) ([]archeryclient.InstanceInfo, error) {
		return []archeryclient.InstanceInfo{{ID: 1, InstanceName: "prod", DBType: "mysql"}}, nil
	})
	t.Cleanup(func() { SetArcheryClientFactory(nil) })
	t.Cleanup(func() { SetArcheryInstanceListFactory(nil) })
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

func TestArcheryQueryNode_InitAllowsRuntimeInstanceID(t *testing.T) {
	n := &ArcheryQueryNode{}
	if err := n.Init(types.Config{}, types.Configuration{}); err != nil {
		t.Fatalf("runtime instanceId should be allowed: %v", err)
	}
	if err := n.Init(types.Config{}, types.Configuration{"instanceId": 1}); err != nil {
		t.Fatalf("unexpected init error: %v", err)
	}
	if n.config.LimitNum != 100 {
		t.Fatalf("expected default limitNum=100, got %d", n.config.LimitNum)
	}
}

func TestArcherySchemaNode_InitValidatesResource(t *testing.T) {
	n := &ArcherySchemaNode{}
	if err := n.Init(types.Config{}, types.Configuration{"instanceId": 1, "resource": "bogus"}); err == nil {
		t.Fatal("expected error for invalid resource")
	}
	// databases 不需要 dbName。
	if err := n.Init(types.Config{}, types.Configuration{"instanceId": 1, "resource": "databases"}); err != nil {
		t.Fatalf("databases should not require dbName: %v", err)
	}
	// schemas/tables 的 dbName 可由运行时消息提供。
	if err := n.Init(types.Config{}, types.Configuration{"resource": "schemas"}); err != nil {
		t.Fatalf("schemas should allow runtime dbName: %v", err)
	}
	if err := n.Init(types.Config{}, types.Configuration{"resource": "tables"}); err != nil {
		t.Fatalf("tables should allow runtime dbName: %v", err)
	}
	// columns 的 dbName/tableName 可由运行时消息提供。
	if err := n.Init(types.Config{}, types.Configuration{"resource": "columns"}); err != nil {
		t.Fatalf("columns should allow runtime names: %v", err)
	}
	if err := n.Init(types.Config{}, types.Configuration{"instanceId": 1, "resource": "tables", "dbName": "orders"}); err != nil {
		t.Fatalf("unexpected init error: %v", err)
	}
	if err := n.Init(types.Config{}, types.Configuration{"instanceId": 1, "resource": "columns", "dbName": "orders", "tableName": "t1"}); err != nil {
		t.Fatalf("unexpected init error for columns: %v", err)
	}
}

func TestArcherySchemaNode_InitAllowsRuntimeInstanceForInstances(t *testing.T) {
	n := &ArcherySchemaNode{}
	if err := n.Init(types.Config{}, types.Configuration{"resource": "instances"}); err != nil {
		t.Fatalf("instances should allow runtime/default connection resolution: %v", err)
	}
}

func TestMsgParamReadsNumericInstanceIDFromMessage(t *testing.T) {
	msg := types.NewMsg(0, "", types.JSON, types.NewMetadata(), `{"instanceId":12}`)
	if got := msgParam(msg, "instanceId", ""); got != "12" {
		t.Fatalf("expected numeric instanceId to be normalized, got %q", got)
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
	       "configuration": {"instanceId": 1, "dbName": "orders"}}
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

func TestArcheryQueryNode_OnMsgUsesRuntimeInstanceID(t *testing.T) {
	srv := fakeArcheryServer(t)
	defer srv.Close()
	useFakeFactory(t, srv)
	dsl := `{"ruleChain":{"id":"chain_archery_q_runtime","root":true},"metadata":{"nodes":[
		{"id":"q1","type":"archeryQuery","configuration":{"sql":"SELECT 1"}}
	],"connections":[]}}`
	res, err := runDSLForTest("chain_archery_q_runtime", []byte(dsl), "JSON", `{"instanceId":1}`, nil)
	if err != nil || res.Err != nil {
		t.Fatalf("runtime instance query failed: err=%v runErr=%v", err, res.Err)
	}
}

func TestArcheryQueryNode_MetadataInstanceOverridesMessage(t *testing.T) {
	srv := fakeArcheryServer(t)
	defer srv.Close()
	useFakeFactory(t, srv)
	dsl := `{"ruleChain":{"id":"chain_archery_q_metadata","root":true},"metadata":{"nodes":[
		{"id":"q1","type":"archeryQuery","configuration":{"sql":"SELECT 1"}}
	],"connections":[]}}`
	res, err := runDSLForTest("chain_archery_q_metadata", []byte(dsl), "JSON",
		`{"instanceId":2}`, map[string]string{"instanceId": "1"})
	if err != nil || res.Err != nil {
		t.Fatalf("metadata instance should override message: err=%v runErr=%v", err, res.Err)
	}
}

func TestArcheryQueryNode_OnMsgRejectsUnknownInstance(t *testing.T) {
	srv := fakeArcheryServer(t)
	defer srv.Close()
	useFakeFactory(t, srv)
	dsl := `{"ruleChain":{"id":"chain_archery_q_bad_instance","root":true},"metadata":{"nodes":[
		{"id":"q1","type":"archeryQuery","configuration":{"sql":"SELECT 1"}}
	],"connections":[]}}`
	res, err := runDSLForTest("chain_archery_q_bad_instance", []byte(dsl), "JSON",
		`{"instanceId":2}`, nil)
	if err != nil {
		t.Fatalf("unknown instance run error: %v", err)
	}
	if res.Err == nil {
		t.Fatal("unknown instance should fail")
	}
}

func TestArcheryQueryNode_OnMsgFailureWhenNoSQL(t *testing.T) {
	srv := fakeArcheryServer(t)
	defer srv.Close()
	useFakeFactory(t, srv)

	dsl := `{
	  "ruleChain": {"id": "chain_archery_q_fail", "root": true},
	  "metadata": {
	    "nodes": [{"id": "q1", "type": "archeryQuery", "configuration": {"instanceId": 1}}],
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
	       "configuration": {"instanceId": 1, "resource": "tables", "dbName": "orders"}}
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

func TestArcherySchemaNode_OnMsgListsDatabases(t *testing.T) {
	srv := fakeArcheryServer(t)
	defer srv.Close()
	useFakeFactory(t, srv)
	dsl := `{"ruleChain":{"id":"chain_archery_databases","root":true},"metadata":{"nodes":[
		{"id":"s1","type":"archerySchema","configuration":{"instanceId":1,"resource":"databases"}}
	],"connections":[]}}`
	res, err := runDSLForTest("chain_archery_databases", []byte(dsl), "JSON", `{}`, nil)
	if err != nil || res.Err != nil {
		t.Fatalf("database list failed: err=%v runErr=%v", err, res.Err)
	}
	if !strings.Contains(res.Output, `"resource":"databases"`) {
		t.Fatalf("unexpected database output: %q", res.Output)
	}
}

func TestArcherySchemaNode_OnMsgListsColumnsFromRuntimeFields(t *testing.T) {
	srv := fakeArcheryServer(t)
	defer srv.Close()
	useFakeFactory(t, srv)
	dsl := `{"ruleChain":{"id":"chain_archery_columns","root":true},"metadata":{"nodes":[
		{"id":"s1","type":"archerySchema","configuration":{"resource":"columns"}}
	],"connections":[]}}`
	res, err := runDSLForTest("chain_archery_columns", []byte(dsl), "JSON",
		`{"instanceId":1,"dbName":"orders","tableName":"t1"}`, nil)
	if err != nil || res.Err != nil {
		t.Fatalf("column list failed: err=%v runErr=%v", err, res.Err)
	}
	if !strings.Contains(res.Output, `"resource":"columns"`) {
		t.Fatalf("unexpected column output: %q", res.Output)
	}
}

func TestArcherySchemaNode_OnMsgFailsWhenRuntimeFieldMissing(t *testing.T) {
	srv := fakeArcheryServer(t)
	defer srv.Close()
	useFakeFactory(t, srv)
	dsl := `{"ruleChain":{"id":"chain_archery_missing_table","root":true},"metadata":{"nodes":[
		{"id":"s1","type":"archerySchema","configuration":{"resource":"tables"}}
	],"connections":[]}}`
	res, err := runDSLForTest("chain_archery_missing_table", []byte(dsl), "JSON", `{"instanceId":1}`, nil)
	if err != nil {
		t.Fatalf("missing field run error: %v", err)
	}
	if res.Err == nil || !strings.Contains(res.Err.Error(), "dbName") {
		t.Fatalf("expected dbName failure, got %v", res.Err)
	}
}

func TestArcherySchemaNode_OnMsgListsInstances(t *testing.T) {
	srv := fakeArcheryServer(t)
	defer srv.Close()
	useFakeFactory(t, srv)
	dsl := `{"ruleChain":{"id":"chain_archery_instances","root":true},"metadata":{"nodes":[
		{"id":"s1","type":"archerySchema","configuration":{"resource":"instances"}}
	],"connections":[]}}`
	res, err := runDSLForTest("chain_archery_instances", []byte(dsl), "JSON", `{}`, nil)
	if err != nil || res.Err != nil {
		t.Fatalf("instance list failed: err=%v runErr=%v", err, res.Err)
	}
	var out map[string]any
	if err := json.Unmarshal([]byte(res.Output), &out); err != nil {
		t.Fatalf("output not JSON: %v", err)
	}
	if out["count"].(float64) != 1 {
		t.Fatalf("expected one instance, got %v", out["count"])
	}
}

func TestArcheryMCPChain_RunDSLDispatchesAction(t *testing.T) {
	srv := fakeArcheryServer(t)
	defer srv.Close()
	useFakeFactory(t, srv)
	dsl := []byte(`{"ruleChain":{"id":"chain_archery_mcp_dispatch","root":true},"metadata":{"firstNodeIndex":0,"nodes":[
		{"id":"route","type":"switch","configuration":{"cases":[
			{"case":"msg.action == \"listInstances\"","then":"instances"}
		]}},
		{"id":"instances","type":"archerySchema","configuration":{"resource":"instances"}},
		{"id":"failure","type":"jsTransform","configuration":{"jsScript":"throw new Error('unsupported action');"}}
	],"connections":[
		{"fromId":"route","toId":"instances","type":"instances"},
		{"fromId":"route","toId":"failure","type":"Default"}
	]}}`)
	res, err := rulegokit.RunDSL("chain_archery_mcp_dispatch", dsl, "JSON", `{"action":"listInstances"}`, nil)
	if err != nil || res.Err != nil {
		t.Fatalf("dispatch failed: err=%v runErr=%v", err, res.Err)
	}
	if !strings.Contains(res.Output, `"resource":"instances"`) {
		t.Fatalf("expected instances output, got %q", res.Output)
	}
	unknown, err := rulegokit.RunDSL("chain_archery_mcp_dispatch_unknown", dsl, "JSON", `{"action":"unknown"}`, nil)
	if err != nil {
		t.Fatalf("unknown action run error: %v", err)
	}
	if unknown.Err == nil {
		t.Fatal("unknown action should fail through Default branch")
	}
}

func TestArcherySchemaNode_ColumnsAllowsRuntimeTable(t *testing.T) {
	n := &ArcherySchemaNode{}
	err := n.Init(types.Config{}, types.Configuration{
		"resource": "columns",
	})
	if err != nil {
		t.Fatalf("expected runtime tableName to be allowed, got %v", err)
	}
}
