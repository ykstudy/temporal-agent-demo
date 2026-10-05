<script setup lang="ts">
import { computed, ref } from 'vue'
import type { Part, TextPart, ChoicePart, DiffPart, ActionPart, ComponentPart, ProposalDiffPayload, ProgressPayload, ClarificationPayload, ArtifactManifestPayload, VerificationReportPayload, RunSnapshot, ControlRequest, ObjectReference } from '../contracts/types.generated'
import { validate } from '../contracts/runtime'
import MarkdownMessage from '../components/MarkdownMessage.vue'
import ProposalCard from './ProposalCard.vue'
const props = defineProps<{ part: Part; run: RunSnapshot; busy: boolean }>()
const emit = defineEmits<{ action: [intent: ControlRequest]; choose: [content: string]; inspect: [reference: ObjectReference, schema: string] }>()
const text = computed(() => props.part.type === 'text' ? validate<TextPart>('TextPart', props.part) : null)
const choice = computed(() => props.part.type === 'choice' ? validate<ChoicePart>('ChoicePart', props.part) : null)
const diff = computed(() => props.part.type === 'diff' ? validate<DiffPart>('DiffPart', props.part) : null)
const action = computed(() => props.part.type === 'action' ? validate<ActionPart>('ActionPart', props.part) : null)
const component = computed(() => props.part.type === 'component' ? validate<ComponentPart>('ComponentPart', props.part) : null)
const proposal = computed(() => component.value?.component_type === 'proposal_diff' ? validate<ProposalDiffPayload>('ProposalDiffPayload', component.value.payload) : null)
const progress = computed(() => component.value?.component_type === 'progress' ? validate<ProgressPayload>('ProgressPayload', component.value.payload) : null)
const clarification = computed(() => component.value?.component_type === 'clarification' ? validate<ClarificationPayload>('ClarificationPayload', component.value.payload) : null)
const manifest = computed(() => component.value?.component_type === 'artifact_manifest' ? validate<ArtifactManifestPayload>('ArtifactManifestPayload', component.value.payload) : null)
const verification = computed(() => component.value?.component_type === 'verification_report' ? validate<VerificationReportPayload>('VerificationReportPayload', component.value.payload) : null)
const selected = ref<string[]>([])
const allowed = computed(() => !props.busy && !['DONE', 'FAILED', 'CANCELLED'].includes(props.run.state))
function choose() { if (choice.value) emit('choose', choice.value.choices.filter(c => selected.value.includes(c.choice_id)).map(c => c.label).join('，')) }
</script>
<template>
  <MarkdownMessage v-if="text?.format === 'markdown'" :content="text.text" />
  <p v-else-if="text" class="v2-text">{{ text.text }}</p>
  <ProposalCard v-else-if="proposal" :reference="proposal.proposal_ref" :run="run" :busy="busy" @action="emit('action', $event)" />
  <p v-else-if="progress" class="v2-progress-note">{{ progress.summary }}</p>
  <p v-else-if="clarification" class="v2-text">{{ clarification.question }}</p>
  <button v-else-if="manifest" @click="emit('inspect', manifest.manifest_ref, 'ArtifactManifest')">{{ manifest.summary }} · {{ manifest.artifact_count }} 个文件</button>
  <button v-else-if="verification" @click="emit('inspect', verification.report_ref, 'VerificationReport')">{{ verification.summary }} · {{ verification.result }}</button>
  <fieldset v-else-if="choice" class="v2-choices" :disabled="!allowed"><legend>{{ choice.prompt }}</legend><label v-for="option in choice.choices" :key="option.choice_id"><input v-if="choice.selection_mode === 'multiple'" v-model="selected" type="checkbox" :value="option.choice_id"><input v-else type="radio" :name="part.part_id" :checked="selected.includes(option.choice_id)" @change="selected = [option.choice_id]">{{ option.label }}<small>{{ option.description }}</small></label><button :disabled="!selected.length" @click="choose">填入我的选择</button></fieldset>
  <section v-else-if="diff" class="v2-diff-block"><p>{{ diff.summary }}</p><p class="hash">来源 v{{ diff.object_ref.source_version }} · {{ diff.object_ref.hash }}</p><details v-for="change in diff.changes" :key="change.path"><summary>{{ change.operation }} · {{ change.path }}</summary><div class="v2-diff"><pre v-if="change.before !== null">{{ change.before }}</pre><pre v-if="change.after !== null">{{ change.after }}</pre></div></details></section>
  <button v-else-if="action" :disabled="!allowed || action.intent.target.object_id !== run.run_id || action.intent.target.source_version !== run.source_version || (action.intent.required_proposal_ref && action.intent.required_proposal_ref.hash !== run.proposal?.snapshot_hash)" @click="emit('action', action.intent)">{{ action.label }}</button>
  <p v-else class="v2-text" data-safe-fallback>{{ part.fallback_text }}</p>
</template>
