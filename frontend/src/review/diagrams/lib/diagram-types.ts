export interface DiagramSelection {
  id: string
  label: string
  type?: string
  description?: string
  details?: string[]
  value?: string | number
}

export interface DiagramBaseData {
  id: string
  title: string
  description: string
}

export interface HierarchyNode {
  id: string
  label: string
  subtitle?: string
  color?: string
  children?: HierarchyNode[]
}

export interface HierarchyData extends DiagramBaseData { root: HierarchyNode }
export interface RelationshipNode { id: string; label: string; type: string; description?: string; color?: string }
export interface RelationshipEdge { id: string; source: string; target: string; label: string }
export interface RelationshipData extends DiagramBaseData { nodes: RelationshipNode[]; edges: RelationshipEdge[] }
export interface TimelineEvent { id: string; date: string; label: string; description: string; status: 'complete' | 'current' | 'planned' }
export interface TimelineData extends DiagramBaseData { events: TimelineEvent[] }
export interface GanttTask { id: string; label: string; owner: string; start: string; end: string; progress: number; dependsOn?: string[]; color?: string }
export interface GanttData extends DiagramBaseData { tasks: GanttTask[] }
export interface PertTask { id: string; label: string; duration: number; dependsOn: string[] }
export interface PertData extends DiagramBaseData { tasks: PertTask[] }
