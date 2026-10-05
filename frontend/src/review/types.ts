export interface Source { id: string; path: string; sha256: string; text: string }
export interface Binding { label: string; description?: string; details: string[]; source_ids: string[]; status?: string; scope?: string }
export interface ReviewModel {
  id: string; title: string; diagram_type: string; category: string; description: string;
  scope: 'design' | 'current' | 'evidence'; status: string; source_ids: string[];
  semantics: string[]; data: Record<string, unknown>; bindings: Record<string, Binding>;
}
export interface ReviewBundle {
  schema_version: 1; generated_at: string;
  baseline: { source_sha256: string; invariants_sha256: string; contract_version: string };
  sources: Source[]; models: ReviewModel[];
}
export interface CoverageRow { id: string; label: string; priority: string; description: string; case_ids: string[] }
export interface CoverageColumn { id: string; label: string }
export interface CoverageCell { id: string; row: string; column: string; status: string; label: string; detail: string; case_ids: string[]; evidence_ids: string[] }
export interface CoverageData { id: string; title: string; description: string; rows: CoverageRow[]; columns: CoverageColumn[]; cells: CoverageCell[] }
