export const effortLevels = ['low', 'medium', 'high'] as const
export type EffortLevel = (typeof effortLevels)[number]

export interface EffortModelsDto {
  low?: string
  medium?: string
  high?: string
  default?: EffortLevel
}

export interface DetectedEffortFamily {
  baseId: string
  effortModels: EffortModelsDto
  sourceIds: string[]
}

export function normalizeEffortModels(
  input?: EffortModelsDto | null,
): EffortModelsDto | undefined {
  if (!input) return undefined
  const low = input.low?.trim() ?? ''
  const medium = input.medium?.trim() ?? ''
  const high = input.high?.trim() ?? ''
  if (!low && !medium && !high) return undefined
  let defaultLevel: EffortLevel =
    input.default === 'low' || input.default === 'medium' || input.default === 'high'
      ? input.default
      : medium
        ? 'medium'
        : high
          ? 'high'
          : 'low'
  if (
    (defaultLevel === 'low' && !low) ||
    (defaultLevel === 'medium' && !medium) ||
    (defaultLevel === 'high' && !high)
  ) {
    defaultLevel = medium ? 'medium' : high ? 'high' : 'low'
  }
  return {
    ...(low ? { low } : {}),
    ...(medium ? { medium } : {}),
    ...(high ? { high } : {}),
    default: defaultLevel,
  }
}

export function hasEffortModels(input?: EffortModelsDto | null): boolean {
  return normalizeEffortModels(input) !== undefined
}

export function effortModelsCount(input?: EffortModelsDto | null): number {
  const normalized = normalizeEffortModels(input)
  if (!normalized) return 0
  return [normalized.low, normalized.medium, normalized.high].filter(Boolean).length
}

const tierSuffixPattern = /^(.+)-(low|medium|high)$/iu

export function detectEffortFamilies(modelIds: readonly string[]): DetectedEffortFamily[] {
  const families = new Map<
    string,
    { low?: string; medium?: string; high?: string; sourceIds: Set<string> }
  >()
  const trimmedIds = modelIds.map((id) => id.trim()).filter(Boolean)
  const idSet = new Set(trimmedIds)

  for (const id of trimmedIds) {
    const match = tierSuffixPattern.exec(id)
    if (!match) continue
    const baseId = match[1]!
    const tier = match[2]!.toLowerCase() as EffortLevel
    const entry = families.get(baseId) ?? { sourceIds: new Set<string>() }
    entry[tier] = id
    entry.sourceIds.add(id)
    if (idSet.has(`${baseId}-tiered`)) {
      entry.sourceIds.add(`${baseId}-tiered`)
    }
    families.set(baseId, entry)
  }

  const results: DetectedEffortFamily[] = []
  for (const [baseId, entry] of families.entries()) {
    const count = [entry.low, entry.medium, entry.high].filter(Boolean).length
    if (count < 2) continue
    const effortModels = normalizeEffortModels({
      low: entry.low,
      medium: entry.medium,
      high: entry.high,
      default: entry.medium ? 'medium' : entry.high ? 'high' : 'low',
    })
    if (!effortModels) continue
    results.push({
      baseId,
      effortModels,
      sourceIds: [...entry.sourceIds],
    })
  }
  return results
}
