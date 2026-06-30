// Package zaprpc is a stub for the native ZAP RPC surface that mirrors
// the HTTP /v1/notify/* routes.
//
// In the launch slice we ship HTTP only. ZAP procedures (notify.Send,
// notify.Message, notify.Providers, …) land once the corresponding
// hanzoai/zap server-side helpers stabilize in hanzoai/base or as a
// separate hanzoai/zap-server module. This file pins the package
// identifier so the structure compiles and consumers can import the
// Procedure constants today.
package zaprpc

// Procedure names follow the dot-prefixed convention used elsewhere in
// the Hanzo stack (kms.Get, tasks.ExecuteWorkflow, …). One opcode per
// HTTP verb-path; the routes/* package is the authoritative
// implementation, and ZAP procedures dispatch to the same handlers.
const (
	ProcedureSend          = "notify.Send"
	ProcedureMessage       = "notify.Message"
	ProcedureMessages      = "notify.Messages"
	ProcedureProviders     = "notify.Providers"
	ProcedureTemplates     = "notify.Templates"
	ProcedureEvents        = "notify.Events"
	ProcedureMetering      = "notify.Metering"
)

// Cognitive (cog.*) procedures follow HIP-0114 §"Reference Implementation"
// (ZAP Inter-VM Cognitive Transport). They carry intents, receipts, and
// agent/operator artifacts between the C-role sidecar, A-Chain runtime,
// and signer firewall. ZAP is transport only — none of these procedures
// is a consensus input on its own; certification is by committed outbox
// entries, Merkle proofs against `receipt_root`, or signer policy.
const (
	ProcedureCogSubmitIntent   = "cog.SubmitIntent"
	ProcedureCogDeliverReceipt = "cog.DeliverReceipt"
	ProcedureCogAgentArtifact  = "cog.AgentArtifact"
	ProcedureCogOperatorAction = "cog.OperatorAction"
	ProcedureCogBridgeHealth   = "cog.BridgeHealth"
	ProcedureCogSimRequest     = "cog.SimRequest"
	ProcedureCogSimResult      = "cog.SimResult"
)
