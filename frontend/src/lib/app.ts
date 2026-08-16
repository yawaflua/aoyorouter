import type { ProviderType } from './models/providers'

export type Section = 'keys' | 'providers' | 'logs'
export type Dialog = 'key' | 'provider' | 'secret' | 'delete-key' | 'delete-provider' | null

export interface SectionDefinition {
  id: Section
  icon: 'key' | 'provider' | 'logs'
  label: string
  title: string
  description: string
}

export const sections: SectionDefinition[] = [
  {
    id: 'keys',
    icon: 'key',
    label: 'API keys',
    title: 'API keys',
    description: 'Create and revoke credentials used by your applications.',
  },
  {
    id: 'providers',
    icon: 'provider',
    label: 'Providers',
    title: 'Providers',
    description: 'Connect the model services available through your router.',
  },
  {
    id: 'logs',
    icon: 'logs',
    label: 'Logs',
    title: 'Request logs',
    description: 'Inspect requests routed through each provider.',
  },
]

export const providerLabels: Record<ProviderType, string> = {
  PROVIDER_TYPE_CUSTOM: 'OpenAI-compatible',
  PROVIDER_TYPE_OPENAI: 'OpenAI Codex',
  PROVIDER_TYPE_ANTHROPIC: 'Anthropic',
  PROVIDER_TYPE_KIMI: 'Kimi',
  PROVIDER_TYPE_GROK: 'xAI Grok',
  PROVIDER_TYPE_ANTIGRAVITY: 'Google Antigravity',
}

export const providerOptions = Object.entries(providerLabels).map(([value, label]) => ({
  value: value as ProviderType,
  label,
}))

export function getSection(id: Section): SectionDefinition {
  return sections.find((section) => section.id === id) ?? sections[0]
}
