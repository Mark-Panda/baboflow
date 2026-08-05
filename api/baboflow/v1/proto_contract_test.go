package v1

import (
	"testing"

	annotations "google.golang.org/genproto/googleapis/api/annotations"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
)

func TestProtoServices(t *testing.T) {
	files := map[string]string{
		"AuthService":      string(File_baboflow_v1_auth_proto.Services().ByName("AuthService").FullName()),
		"LLMService":       string(File_baboflow_v1_llm_proto.Services().ByName("LLMService").FullName()),
		"ArcheryService":   string(File_baboflow_v1_archery_proto.Services().ByName("ArcheryService").FullName()),
		"ComponentService": string(File_baboflow_v1_component_proto.Services().ByName("ComponentService").FullName()),
		"RuleChainService": string(File_baboflow_v1_rulechain_proto.Services().ByName("RuleChainService").FullName()),
		"AgentService":     string(File_baboflow_v1_agent_proto.Services().ByName("AgentService").FullName()),
		"SkillService":     string(File_baboflow_v1_skill_proto.Services().ByName("SkillService").FullName()),
		"McpService":       string(File_baboflow_v1_mcp_proto.Services().ByName("McpService").FullName()),
		"BoardService":     string(File_baboflow_v1_board_proto.Services().ByName("BoardService").FullName()),
		"AuditService":     string(File_baboflow_v1_audit_proto.Services().ByName("AuditService").FullName()),
		"CronService":      string(File_baboflow_v1_cron_proto.Services().ByName("CronService").FullName()),
	}
	for service, name := range files {
		if name == "" {
			t.Fatalf("missing service %s", service)
		}
	}
}

func TestProtoHTTPPaths(t *testing.T) {
	cases := []struct {
		name string
		path string
	}{
		{"auth login", httpPath(File_baboflow_v1_auth_proto, "AuthService", "Login")},
		{"archery connections", httpPath(File_baboflow_v1_archery_proto, "ArcheryService", "ListConnections")},
		{"agents", httpPath(File_baboflow_v1_agent_proto, "AgentService", "List")},
		{"skills", httpPath(File_baboflow_v1_skill_proto, "SkillService", "List")},
		{"boards", httpPath(File_baboflow_v1_board_proto, "BoardService", "List")},
		{"audit logs", httpPath(File_baboflow_v1_audit_proto, "AuditService", "List")},
		{"cron jobs", httpPath(File_baboflow_v1_cron_proto, "CronService", "List")},
		{"rule chains", httpPath(File_baboflow_v1_rulechain_proto, "RuleChainService", "List")},
	}
	want := []string{
		"/api/v1/auth/login",
		"/api/v1/archery/connections",
		"/api/v1/agents",
		"/api/v1/skills",
		"/api/v1/boards",
		"/api/v1/audit-logs",
		"/api/v1/cron-jobs",
		"/api/v1/rule-chains",
	}
	for i, tc := range cases {
		if tc.path != want[i] {
			t.Fatalf("%s path = %q, want %q", tc.name, tc.path, want[i])
		}
	}
}

func TestEveryRPCExposesUniqueHTTPBinding(t *testing.T) {
	files := []protoreflect.FileDescriptor{
		File_baboflow_v1_auth_proto, File_baboflow_v1_llm_proto,
		File_baboflow_v1_archery_proto, File_baboflow_v1_component_proto,
		File_baboflow_v1_rulechain_proto, File_baboflow_v1_agent_proto,
		File_baboflow_v1_skill_proto, File_baboflow_v1_mcp_proto,
		File_baboflow_v1_board_proto, File_baboflow_v1_audit_proto,
		File_baboflow_v1_cron_proto,
	}
	seen := map[string]bool{}
	for _, file := range files {
		for i := 0; i < file.Services().Len(); i++ {
			service := file.Services().Get(i)
			for j := 0; j < service.Methods().Len(); j++ {
				method := service.Methods().Get(j)
				rule, ok := proto.GetExtension(method.Options(), annotations.E_Http).(*annotations.HttpRule)
				if !ok || rule == nil || httpVerb(rule) == "" || httpRulePath(rule) == "" {
					t.Fatalf("%s.%s has no complete HTTP binding", service.Name(), method.Name())
				}
				key := httpVerb(rule) + " " + httpRulePath(rule)
				if seen[key] {
					t.Fatalf("duplicate HTTP binding %s", key)
				}
				seen[key] = true
				if method.IsStreamingClient() || method.IsStreamingServer() {
					t.Fatalf("%s.%s must not stream", service.Name(), method.Name())
				}
			}
		}
	}
}

func TestRuleChainRunPersistenceContract(t *testing.T) {
	run := File_baboflow_v1_rulechain_proto.Messages().ByName("RuleChainRun")
	if run == nil {
		t.Fatal("missing RuleChainRun message")
	}
	nodeTrace := run.Fields().ByName("nodeTrace")
	if nodeTrace == nil {
		t.Fatal("missing RuleChainRun.nodeTrace field")
	}
	if nodeTrace.Cardinality() != protoreflect.Repeated || nodeTrace.Kind() != protoreflect.MessageKind {
		t.Fatalf("RuleChainRun.nodeTrace must be a repeated message, got cardinality=%s kind=%s", nodeTrace.Cardinality(), nodeTrace.Kind())
	}
	if got, want := nodeTrace.Message().FullName(), protoreflect.FullName("baboflow.v1.NodeTrace"); got != want {
		t.Fatalf("RuleChainRun.nodeTrace message = %q, want %q", got, want)
	}
	taskID := run.Fields().ByName("taskId")
	if taskID == nil {
		t.Fatal("missing RuleChainRun.taskId field")
	}
	if !taskID.HasPresence() || !taskID.HasOptionalKeyword() {
		t.Fatal("RuleChainRun.taskId must have proto3 optional presence")
	}
}

func httpPath(file protoreflect.FileDescriptor, service, method string) string {
	options := file.Services().ByName(protoreflect.Name(service)).
		Methods().ByName(protoreflect.Name(method)).Options()
	rule, ok := proto.GetExtension(options, annotations.E_Http).(*annotations.HttpRule)
	if !ok || rule == nil {
		return ""
	}
	switch pattern := rule.GetPattern().(type) {
	case *annotations.HttpRule_Get:
		return pattern.Get
	case *annotations.HttpRule_Post:
		return pattern.Post
	case *annotations.HttpRule_Put:
		return pattern.Put
	case *annotations.HttpRule_Delete:
		return pattern.Delete
	default:
		return ""
	}
}

func httpRulePath(rule *annotations.HttpRule) string {
	switch pattern := rule.GetPattern().(type) {
	case *annotations.HttpRule_Get:
		return pattern.Get
	case *annotations.HttpRule_Post:
		return pattern.Post
	case *annotations.HttpRule_Put:
		return pattern.Put
	case *annotations.HttpRule_Delete:
		return pattern.Delete
	default:
		return ""
	}
}

func httpVerb(rule *annotations.HttpRule) string {
	switch rule.GetPattern().(type) {
	case *annotations.HttpRule_Get:
		return "GET"
	case *annotations.HttpRule_Post:
		return "POST"
	case *annotations.HttpRule_Put:
		return "PUT"
	case *annotations.HttpRule_Delete:
		return "DELETE"
	default:
		return ""
	}
}
