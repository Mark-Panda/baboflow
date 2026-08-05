package biz

import (
	"encoding/json"
	"time"

	v1 "baboflow/api/baboflow/v1"
	"baboflow/internal/biz/rulegokit"
	"baboflow/internal/data/po"

	"google.golang.org/protobuf/types/known/structpb"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

const protoTimeLayout = "2006-01-02T15:04:05Z07:00"

// The following helpers keep persistence-to-proto conversion in the biz layer.
func ToProtoLLMModel(in po.LLMModel) *v1.LLMModel {
	return &v1.LLMModel{Id: in.ID, ProviderId: in.ProviderID, Model: in.Model, Alias: in.Alias, Temperature: in.Temperature, MaxTokens: int32(in.MaxTokens), IsDefault: in.IsDefault, Capability: ProtoJSONStruct(in.Capability), Enabled: wrapperspb.Bool(in.Enabled)}
}

func ToProtoCronJob(in *po.CronJob) *v1.CronJob {
	out := &v1.CronJob{Id: in.ID, Name: in.Name, TargetType: in.TargetType, TargetId: in.TargetID, ScheduleType: in.ScheduleType, CronExpr: in.CronExpr, IntervalSec: int64(in.IntervalSec), Payload: ProtoJSONStruct(in.Payload), Enabled: in.Enabled, LastStatus: in.LastStatus, CreatedAt: ProtoTime(in.CreatedAt), UpdatedAt: ProtoTime(in.UpdatedAt)}
	if in.RunAt != nil {
		out.RunAt = ProtoTime(*in.RunAt)
	}
	if in.LastRunAt != nil {
		out.LastRunAt = ProtoTime(*in.LastRunAt)
	}
	return out
}

func ToProtoAgentSession(in *po.AgentSession) *v1.AgentSession {
	return &v1.AgentSession{Id: in.ID, AgentKey: in.AgentKey, ChainId: in.ChainID, Title: in.Title, CreatedAt: in.CreatedAt.Format(protoTimeLayout), UpdatedAt: in.UpdatedAt.Format(protoTimeLayout)}
}

func ToProtoAgentMessage(in *po.AgentMessage) *v1.AgentMessage {
	out := &v1.AgentMessage{Id: in.ID, SessionId: in.SessionID, Role: in.Role, Content: in.Content, SubAgent: in.SubAgent, CreatedAt: in.CreatedAt.Format(protoTimeLayout)}
	_ = json.Unmarshal(in.ToolCalls, &out.ToolCalls)
	_ = json.Unmarshal(in.Attachment, &out.Attachment)
	return out
}

func ToProtoMcpServer(in *po.McpServer) *v1.McpServer {
	args := []string{}
	env := map[string]string{}
	_ = json.Unmarshal(in.Args, &args)
	_ = json.Unmarshal(in.Env, &env)
	out := &v1.McpServer{Id: in.ID, Name: in.Name, Transport: in.Transport, Endpoint: in.Endpoint, Command: in.Command, Args: args, Env: env, Status: in.Status, CreatedAt: ProtoTime(in.CreatedAt), UpdatedAt: ProtoTime(in.UpdatedAt)}
	if in.LastCheckAt != nil {
		out.LastCheckAt = ProtoTime(*in.LastCheckAt)
	}
	return out
}

func ToProtoMcpExposure(in *po.McpExposure) *v1.McpExposure {
	return &v1.McpExposure{Id: in.ID, ChainId: in.ChainID, ToolName: in.ToolName, Description: in.Description, InputSchema: ProtoJSONStruct(in.InputSchema), Enabled: in.Enabled, CreatedAt: ProtoTime(in.CreatedAt), UpdatedAt: ProtoTime(in.UpdatedAt)}
}

func ToProtoBoard(in *po.Board) *v1.Board {
	return &v1.Board{Id: in.ID, Name: in.Name, Description: in.Description, CreatedAt: ProtoTime(in.CreatedAt), UpdatedAt: ProtoTime(in.UpdatedAt)}
}

func ToProtoBoardDetail(in *BoardDetail) *v1.BoardDetail {
	out := &v1.BoardDetail{Id: in.ID, Name: in.Name, Description: in.Description, CreatedAt: ProtoTime(in.CreatedAt), UpdatedAt: ProtoTime(in.UpdatedAt), Columns: make([]*v1.BoardColumn, 0, len(in.Columns))}
	for i := range in.Columns {
		out.Columns = append(out.Columns, ToProtoBoardColumn(&in.Columns[i].BoardColumn, in.Columns[i].Tasks))
	}
	return out
}

func ToProtoBoardColumn(in *po.BoardColumn, tasks []po.Task) *v1.BoardColumn {
	out := &v1.BoardColumn{Id: in.ID, BoardId: in.BoardID, Name: in.Name, Sort: int64(in.Sort), Tasks: make([]*v1.BoardTask, 0, len(tasks))}
	for i := range tasks {
		out.Tasks = append(out.Tasks, ToProtoBoardTask(&tasks[i]))
	}
	return out
}

func ToProtoBoardTask(in *po.Task) *v1.BoardTask {
	out := &v1.BoardTask{Id: in.ID, ColumnId: in.ColumnID, Title: in.Title, Payload: string(in.Payload), Status: in.Status, AssignedChainId: in.AssignedChainID, RetryMax: int32(in.RetryMax), RetryCount: int32(in.RetryCount), TimeoutSec: int32(in.TimeoutSec), Sort: int64(in.Sort), CreatedAt: ProtoTime(in.CreatedAt), UpdatedAt: ProtoTime(in.UpdatedAt)}
	if in.RunID != nil {
		out.RunId = *in.RunID
	}
	var result struct{ Output, Error string }
	if json.Unmarshal(in.Result, &result) == nil && (result.Output != "" || result.Error != "") {
		out.Result = &v1.BoardTaskResult{Output: result.Output, Error: result.Error}
	}
	return out
}

func ToProtoAuditLog(in *po.AuditLog) *v1.AuditLog {
	out := &v1.AuditLog{Id: in.ID, Action: in.Action, TargetType: in.TargetType, TargetId: in.TargetID, Detail: ProtoJSONStruct(in.Detail), Ip: in.IP, CreatedAt: ProtoTime(in.CreatedAt)}
	if in.UserID != nil {
		out.UserId = *in.UserID
	}
	return out
}

func ToProtoRuleChain(in *po.RuleChain) *v1.RuleChain {
	return &v1.RuleChain{Id: in.ID, Name: in.Name, Description: in.Description, InputSchema: ProtoJSONStruct(in.InputSchema), Status: in.Status, Version: int32(in.Version), Source: in.Source, DebugMode: in.DebugMode, Dsl: ProtoJSONStruct(in.DSL), UpdatedAt: in.UpdatedAt.Format(protoTimeLayout), CreatedAt: in.CreatedAt.Format(protoTimeLayout)}
}

func ToProtoRuleChainVersion(in *po.RuleChainVersion) *v1.RuleChainVersion {
	return &v1.RuleChainVersion{Id: in.ID, ChainId: in.ChainID, Version: int32(in.Version), Dsl: ProtoJSONStruct(in.DSL), PublishedAt: in.PublishedAt.Format(protoTimeLayout)}
}

func ToProtoRuleChainRun(in *po.ChainRun) *v1.RuleChainRun {
	out := &v1.RuleChainRun{Id: in.ID, ChainId: in.ChainID, Trigger: in.Trigger, Input: ProtoJSONStruct(in.Input), Status: in.Status, Output: ProtoJSONStruct(in.Output), Error: in.Error, StartedAt: in.StartedAt.Format(protoTimeLayout), NodeTrace: protoPersistedNodeTraces(in.NodeTrace)}
	if in.FinishedAt != nil {
		out.FinishedAt = in.FinishedAt.Format(protoTimeLayout)
	}
	if in.TaskID != nil {
		taskID := *in.TaskID
		out.TaskId = &taskID
	}
	return out
}

func ProtoJSONStruct(in []byte) *structpb.Struct {
	var value map[string]any
	if json.Unmarshal(in, &value) != nil {
		return nil
	}
	out, _ := structpb.NewStruct(value)
	return out
}

func ProtoTime(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.Format(time.RFC3339)
}

func protoPersistedNodeTraces(in []byte) []*v1.NodeTrace {
	var traces []rulegokit.NodeTrace
	if json.Unmarshal(in, &traces) != nil {
		return []*v1.NodeTrace{}
	}
	return ProtoNodeTraces(traces)
}

func ProtoNodeTraces(values []rulegokit.NodeTrace) []*v1.NodeTrace {
	out := make([]*v1.NodeTrace, 0, len(values))
	for _, value := range values {
		out = append(out, &v1.NodeTrace{NodeId: value.NodeID, FlowType: value.FlowType, RelationType: value.RelationType, Data: value.Data, In: value.In, Out: value.Out, Input: protoTraceMessage(value.Input), Output: protoTraceMessage(value.Output), DurationMs: value.DurationMs, Err: value.Err})
	}
	return out
}

func protoTraceMessage(value *rulegokit.TraceMessage) *v1.TraceMessage {
	if value == nil {
		return nil
	}
	return &v1.TraceMessage{Msg: value.Msg, Metadata: value.Metadata, Type: value.Type, DataType: value.DataType}
}
