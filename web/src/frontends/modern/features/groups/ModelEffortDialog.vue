<script setup lang="ts">
import { Layers, Sparkles } from '@lucide/vue'
import { DialogRoot } from 'reka-ui'
import { computed, ref, useId, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import {
  AppButton,
  AppCheckbox,
  AppDialogContent,
  AppDialogHeader,
  AppIcon,
  AppNotice,
  AppSegmentedField,
  AppTextField,
} from '@modern/components/ui'
import {
  detectEffortFamilies,
  normalizeEffortModels,
  type DetectedEffortFamily,
  type EffortLevel,
  type EffortModelsDto,
} from '@shared/effort-models'
import type { GroupDraftModel } from './group-create-rules'

const props = defineProps<{
  models: readonly GroupDraftModel[]
  candidateIds?: readonly string[]
  targetKey?: number
}>()

const emit = defineEmits<{
  close: []
  save: [
    payload: {
      targetKey?: number
      id: string
      alias: string
      effortModels: EffortModelsDto
      removeSourceRows: boolean
    },
  ]
  clear: [targetKey: number]
  applyFamilies: [families: DetectedEffortFamily[]]
}>()

const { t } = useI18n()
const datalistId = useId()

const targetModel = computed(() =>
  props.targetKey !== undefined
    ? props.models.find((item) => item.key === props.targetKey)
    : undefined,
)

const allAvailableIds = computed(() => {
  const set = new Set<string>()
  for (const model of props.models) {
    if (model.id.trim()) set.add(model.id.trim())
  }
  for (const id of props.candidateIds ?? []) {
    if (id.trim()) set.add(id.trim())
  }
  return [...set].sort()
})

const detectedFamilies = computed(() => {
  const unaggregatedIds = props.models
    .filter((model) => !model.effortModels)
    .map((model) => model.id.trim())
    .filter(Boolean)
  return detectEffortFamilies(unaggregatedIds)
})

const baseId = ref('')
const alias = ref('')
const low = ref('')
const medium = ref('')
const high = ref('')
const defaultTier = ref<EffortLevel>('medium')
const removeSourceRows = ref(true)
const error = ref('')

function initFromTarget(): void {
  error.value = ''
  const current = targetModel.value
  if (!current) {
    const firstFamily = detectedFamilies.value[0]
    if (firstFamily) {
      fillFromFamily(firstFamily)
    } else {
      baseId.value = ''
      alias.value = ''
      low.value = ''
      medium.value = ''
      high.value = ''
      defaultTier.value = 'medium'
    }
    return
  }

  alias.value = current.alias
  if (current.effortModels) {
    baseId.value = current.id
    low.value = current.effortModels.low ?? ''
    medium.value = current.effortModels.medium ?? ''
    high.value = current.effortModels.high ?? ''
    defaultTier.value = current.effortModels.default ?? 'medium'
    return
  }

  const trimmed = current.id.trim()
  const tierMatch = /^(.+)-(low|medium|high|tiered)$/iu.exec(trimmed)
  const inferredBase = tierMatch ? tierMatch[1]! : trimmed
  baseId.value = inferredBase
  const available = new Set(allAvailableIds.value)
  low.value =
    available.has(`${inferredBase}-low`) || tierMatch
      ? `${inferredBase}-low`
      : trimmed
        ? `${trimmed}-low`
        : ''
  medium.value =
    available.has(`${inferredBase}-medium`) || tierMatch
      ? `${inferredBase}-medium`
      : trimmed
        ? `${trimmed}-medium`
        : ''
  high.value =
    available.has(`${inferredBase}-high`) || tierMatch
      ? `${inferredBase}-high`
      : trimmed
        ? `${trimmed}-high`
        : ''
  defaultTier.value = 'medium'
}

function fillFromFamily(family: DetectedEffortFamily): void {
  baseId.value = family.baseId
  low.value = family.effortModels.low ?? ''
  medium.value = family.effortModels.medium ?? ''
  high.value = family.effortModels.high ?? ''
  defaultTier.value = family.effortModels.default ?? 'medium'
  error.value = ''
}

watch(() => [props.targetKey, props.models] as const, initFromTarget, { immediate: true })

const tierOptions = computed(() => [
  { value: 'low', label: 'Low' },
  { value: 'medium', label: 'Medium' },
  { value: 'high', label: 'High' },
])

function submit(): void {
  const normalizedBase = baseId.value.trim()
  if (!normalizedBase) {
    error.value = t('groupCreate.modelIDRequired')
    return
  }
  const effortModels = normalizeEffortModels({
    low: low.value,
    medium: medium.value,
    high: high.value,
    default: defaultTier.value,
  })
  if (!effortModels) {
    error.value = t('groupCreate.effortAtLeastOneRequired')
    return
  }
  emit('save', {
    targetKey: props.targetKey,
    id: normalizedBase,
    alias: alias.value.trim(),
    effortModels,
    removeSourceRows: removeSourceRows.value,
  })
}
</script>

<template>
  <DialogRoot
    :open="true"
    @update:open="
      (value) => {
        if (!value) emit('close')
      }
    "
  >
    <AppDialogContent
      :title="t('groupCreate.effortDialogTitle')"
      :description="t('groupCreate.effortDialogDescription')"
    >
      <AppDialogHeader
        :title="t('groupCreate.effortDialogTitle')"
        :description="t('groupCreate.effortDialogDescription')"
        :close-label="t('ui.close')"
        @close="emit('close')"
      />
      <form class="modern-effort-dialog" @submit.prevent="submit">
        <div class="modern-effort-body">
          <div v-if="detectedFamilies.length" class="modern-effort-detected">
            <div class="modern-effort-detected-header">
              <span class="modern-effort-detected-title">
                <AppIcon :icon="Sparkles" size="sm" />
                {{ t('groupCreate.effortDetectedTitle') }}
              </span>
              <AppButton
                size="sm"
                variant="brand"
                :icon="Layers"
                @click="emit('applyFamilies', detectedFamilies)"
              >
                {{ t('groupCreate.effortApplyAllDetected', { count: detectedFamilies.length }) }}
              </AppButton>
            </div>
            <div class="modern-effort-detected-chips">
              <AppButton
                v-for="family in detectedFamilies"
                :key="family.baseId"
                size="sm"
                :variant="baseId.trim() === family.baseId ? 'brand' : 'outline'"
                @click="fillFromFamily(family)"
              >
                {{ family.baseId }}
              </AppButton>
            </div>
          </div>

          <AppNotice v-if="error" tone="danger">{{ error }}</AppNotice>

          <div class="modern-effort-form">
            <div class="modern-effort-row-two">
              <AppTextField
                v-model="baseId"
                :label="t('groupCreate.effortBaseModel')"
                placeholder="claude-opus-5-5"
                size="sm"
                autocomplete="off"
                spellcheck="false"
              />
              <AppTextField
                v-model="alias"
                :label="t('groupCreate.alias')"
                :placeholder="t('groupCreate.aliasOptional')"
                size="sm"
                autocomplete="off"
                spellcheck="false"
              />
            </div>

            <div class="modern-effort-tiers">
              <AppTextField
                v-model="low"
                :label="t('groupCreate.effortLow')"
                :placeholder="baseId.trim() ? `${baseId.trim()}-low` : 'model-low'"
                :list="datalistId"
                size="sm"
                autocomplete="off"
                spellcheck="false"
              />
              <AppTextField
                v-model="medium"
                :label="t('groupCreate.effortMedium')"
                :placeholder="baseId.trim() ? `${baseId.trim()}-medium` : 'model-medium'"
                :list="datalistId"
                size="sm"
                autocomplete="off"
                spellcheck="false"
              />
              <AppTextField
                v-model="high"
                :label="t('groupCreate.effortHigh')"
                :placeholder="baseId.trim() ? `${baseId.trim()}-high` : 'model-high'"
                :list="datalistId"
                size="sm"
                autocomplete="off"
                spellcheck="false"
              />
            </div>

            <datalist :id="datalistId">
              <option v-for="id in allAvailableIds" :key="id" :value="id" />
            </datalist>

            <AppSegmentedField
              v-model="defaultTier"
              :label="t('groupCreate.effortDefault')"
              :options="tierOptions"
              size="sm"
            />

            <AppCheckbox
              v-model="removeSourceRows"
              :label="t('groupCreate.effortRemoveSourceRows')"
            />
          </div>
        </div>

        <footer class="modern-effort-footer">
          <AppButton
            v-if="targetModel?.effortModels && targetKey !== undefined"
            size="sm"
            variant="outline"
            tone="danger"
            @click="emit('clear', targetKey)"
          >
            {{ t('groupCreate.effortClear') }}
          </AppButton>
          <div class="modern-effort-footer-right">
            <AppButton size="sm" variant="ghost" @click="emit('close')">
              {{ t('ui.cancel') }}
            </AppButton>
            <AppButton type="submit" size="sm" variant="primary">
              {{ t('groupCreate.effortSave') }}
            </AppButton>
          </div>
        </footer>
      </form>
    </AppDialogContent>
  </DialogRoot>
</template>

<style scoped>
.modern-effort-dialog {
  display: flex;
  flex: 1;
  min-height: 0;
  flex-direction: column;
}
.modern-effort-body {
  display: flex;
  flex: 1;
  min-width: 0;
  min-height: 0;
  flex-direction: column;
  gap: var(--modern-space-4);
  padding: var(--modern-space-4) var(--modern-space-5);
  overflow-y: auto;
  scrollbar-gutter: var(--modern-scrollbar-gutter);
  overscroll-behavior: contain;
}
.modern-effort-detected {
  display: grid;
  gap: var(--modern-space-2);
  padding: var(--modern-space-3);
  border-radius: var(--modern-radius-panel);
  border: var(--modern-line-width) solid var(--modern-border);
  background: var(--modern-subtle);
}
.modern-effort-detected-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: var(--modern-space-2);
  flex-wrap: wrap;
}
.modern-effort-detected-title {
  display: inline-flex;
  align-items: center;
  gap: var(--modern-space-1-5);
  font-size: var(--modern-font-size-secondary);
  font-weight: var(--modern-weight-medium);
}
.modern-effort-detected-chips {
  display: flex;
  flex-wrap: wrap;
  gap: var(--modern-space-1-5);
}
.modern-effort-form {
  display: grid;
  gap: var(--modern-space-3);
}
.modern-effort-row-two {
  display: grid;
  grid-template-columns: minmax(0, 1.2fr) minmax(0, 1fr);
  gap: var(--modern-space-3);
}
.modern-effort-tiers {
  display: grid;
  gap: var(--modern-space-3);
}
.modern-effort-footer {
  display: flex;
  flex: none;
  align-items: center;
  justify-content: space-between;
  gap: var(--modern-space-2);
  padding: var(--modern-space-3) var(--modern-space-5);
  border-top: var(--modern-line-width) solid var(--modern-border);
}
.modern-effort-footer-right {
  display: flex;
  align-items: center;
  gap: var(--modern-space-2);
  margin-left: auto;
}
@media (max-width: 760px) {
  .modern-effort-body {
    padding: var(--modern-space-3) var(--modern-space-4);
  }
  .modern-effort-footer {
    padding: var(--modern-space-3) var(--modern-space-4);
  }
  .modern-effort-row-two {
    grid-template-columns: minmax(0, 1fr);
  }
}
</style>
