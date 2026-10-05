export interface Check { name: string; passed: boolean; detail: string }
export interface Change { path: string; before: string; after: string }
export interface Proposal {
  id: string; source_version: number; proposal_version: number; snapshot_hash: string;
  definition_hash: string; issued_at: string; expires_at: string; title: string;
  target: { gitlab_url?: string; namespace_id?: string; project_path?: string; ref?: string };
  status: string; changes: Change[]; impact: string[]; checks: Check[];
}
export interface Plan {
  project_name: string; description: string; tech_stack: string; skills: string[];
  database: string; orm: string; auth: string; token_ttl: string;
  log_level: string; log_format: string; log_output: string; log_path: string;
  log_max_size: number; log_max_backups: number; log_max_age: string; log_on_write_error: string;
  repo_provider: string; repo_visibility: string;
}
export interface Message { id: string; role: string; content: string; created_at: string; kind: string; proposal_id?: string }
export interface Run {
  id: string; title: string; state: string; version: number; source_version: number; state_sequence: number;
  plan: Plan; proposal: Proposal | null; messages: Message[];
  steps: { id: string; title: string; state: string; detail: string }[];
  artifacts: { path: string; size: number }[];
  history: { id: string; action: string; version: number; created_at: string; detail: string }[];
  error: string; execution_mode: string; decision_mode: string; delivery_mode: string;
  repo_url: string; pipeline_url: string; required_checks: Check[]; created_at: string; updated_at: string;
}
export interface Bootstrap {
  mode: string; provider: string; execution_mode: string; decision_mode: string; delivery_mode: string;
  gitlab_configured: boolean; temporal_connected: boolean;
  skills: { id: string; name: string; description: string; parameters: Record<string, string[]> }[];
  definition: unknown;
}
