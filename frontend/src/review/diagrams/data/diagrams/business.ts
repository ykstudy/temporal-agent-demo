export interface BusinessItem {
  id: string
  label: string
  description?: string
  details?: string[]
}
export type BusinessPort = 'left' | 'right' | 'top' | 'bottom'
export interface BusinessLink extends BusinessItem {
  source: string
  target: string
  from?: BusinessPort
  to?: BusinessPort
  via?: { x: number; y: number }[]
  labelPosition?: { x: number; y: number }
  tone?: 'accent' | 'success' | 'warning' | 'muted'
}
interface BusinessDocument { id: string; title: string; description: string }
export interface FlowchartNode extends BusinessItem { kind: 'start' | 'end' | 'process' | 'decision' | 'data'; column: number; row: number; caption?: string }
export interface FlowchartData extends BusinessDocument { nodes: FlowchartNode[]; edges: BusinessLink[] }
export interface SwimlaneNode extends BusinessItem { lane: string; column: number; kind?: 'task' | 'event'; caption?: string }
export interface SwimlaneData extends BusinessDocument { lanes: (BusinessItem & { color?: string })[]; nodes: SwimlaneNode[]; edges: BusinessLink[] }
export interface BpmnNode extends BusinessItem { lane: string; column: number; offset?: number; kind: 'start' | 'end' | 'intermediate' | 'task' | 'user-task' | 'service-task' | 'subprocess' | 'exclusive' | 'parallel'; caption?: string; marker?: 'message' | 'timer' }
export interface BpmnData extends BusinessDocument { pool: string; lanes: (BusinessItem & { external?: boolean })[]; nodes: BpmnNode[]; edges: (BusinessLink & { kind?: 'sequence' | 'message' | 'default' })[] }
export interface StateNode extends BusinessItem { kind: 'initial' | 'final' | 'history' | 'state'; column: number; row: number; actions?: string[]; tone?: 'accent' | 'success' | 'warning' }
export interface StateData extends BusinessDocument { nodes: StateNode[]; edges: BusinessLink[]; groups?: (BusinessItem & { members: string[] })[] }
export interface JourneyStage extends BusinessItem { action: string; touchpoints: string[]; emotion: number; emotionLabel: string; pain: string; opportunity: string; duration?: string }
export interface JourneyData extends BusinessDocument { persona: BusinessItem & { role: string; goal: string }; stages: JourneyStage[] }
export interface UseCaseActor extends BusinessItem { side: 'left' | 'right'; kind?: 'person' | 'system' }
export interface UseCaseNode extends BusinessItem { column: number; row: number; primary?: boolean }
export interface UseCaseData extends BusinessDocument { system: string; actors: UseCaseActor[]; useCases: UseCaseNode[]; relations: (BusinessLink & { kind: 'association' | 'include' | 'extend' | 'generalization' })[] }
export interface ActivityNode extends BusinessItem { kind: 'initial' | 'final' | 'action' | 'decision' | 'merge' | 'fork' | 'join'; column: number; row: number; caption?: string }
export interface ActivityData extends BusinessDocument { nodes: ActivityNode[]; edges: BusinessLink[] }

