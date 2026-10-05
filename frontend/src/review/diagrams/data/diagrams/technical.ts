export type TechnicalTone = 'indigo' | 'mint' | 'amber' | 'rose' | 'slate'
export interface TechnicalBase { id: string; title: string; description: string }
export interface TechnicalNode { id: string; label: string; subtitle?: string; description?: string; tone?: TechnicalTone; icon?: string; details?: string[] }
export interface TechnicalConnection { id: string; source: string; target: string; label?: string; description?: string; dashed?: boolean }
export interface ArchitectureData extends TechnicalBase {
  layers: { id: string; label: string; description: string; nodes: TechnicalNode[] }[]
  connections: TechnicalConnection[]
}
export interface DependencyData extends TechnicalBase {
  columns: { id: string; label: string; nodes: (TechnicalNode & { version?: string })[] }[]
  connections: (TechnicalConnection & { kind: 'runtime' | 'build' | 'optional' })[]
}
export interface DeploymentData extends TechnicalBase {
  environments: { id: string; label: string; region: string; hosts: { id: string; label: string; address: string; services: (TechnicalNode & { image: string; replicas: number; port: string })[] }[] }[]
  connections: TechnicalConnection[]
}
export interface NetworkData extends TechnicalBase {
  zones: { id: string; label: string; subnet: string; devices: (TechnicalNode & { address: string; device: 'cloud' | 'firewall' | 'router' | 'switch' | 'server' | 'database' | 'workstation' })[] }[]
  connections: (TechnicalConnection & { bandwidth?: string })[]
}
export interface SequenceData extends TechnicalBase {
  participants: (TechnicalNode & { kind: 'actor' | 'service' | 'database' })[]
  messages: { id: string; from: string; to: string; label: string; kind: 'sync' | 'async' | 'return'; description?: string }[]
  activations: { participant: string; start: number; end: number }[]
  notes?: { after: number; label: string; from: string; to: string }[]
}
export interface EntityField { name: string; type: string; key?: 'PK' | 'FK'; nullable?: boolean; description?: string }
export type Cardinality = 'one' | 'many' | 'optional-one' | 'optional-many'
export interface ErData extends TechnicalBase {
  entities: (TechnicalNode & { table: string; fields: EntityField[] })[]
  relationships: (TechnicalConnection & { sourceCardinality: Cardinality; targetCardinality: Cardinality })[]
}
export interface DataFlowData extends TechnicalBase {
  nodes: (TechnicalNode & { kind: 'external' | 'process' | 'store'; number?: string })[]
  connections: TechnicalConnection[]
}
export interface LineageData extends TechnicalBase {
  stages: { id: string; label: string; description: string; nodes: (TechnicalNode & { fields: string[]; operation?: string })[] }[]
  connections: TechnicalConnection[]
}
export interface ClassData extends TechnicalBase {
  classes: (TechnicalNode & { stereotype?: string; attributes: string[]; methods: string[]; abstract?: boolean })[]
  relationships: (TechnicalConnection & { kind: 'association' | 'dependency' | 'inheritance' | 'realization' | 'aggregation' | 'composition'; sourceMultiplicity?: string; targetMultiplicity?: string })[]
}

