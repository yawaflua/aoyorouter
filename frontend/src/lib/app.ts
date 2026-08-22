import type { ProviderType } from './models/providers'

export type Section = 'keys' | 'providers' | 'proxies' | 'logs'
export type Dialog = 'key' | 'edit-key' | 'provider' | 'edit-provider' | 'secret' | 'delete-key' | 'delete-provider' | 'edit-proxy' | null

export interface SectionDefinition {
  id: Section
  icon: 'key' | 'provider' | 'proxy' | 'logs'
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
    id: 'proxies',
    icon: 'proxy',
    label: 'Proxies',
    title: 'Live proxies',
    description: 'Inspect proxy instances currently running in the router.',
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
  PROVIDER_TYPE_OPENCODE_ZEN: 'OpenCode Zen',
  PROVIDER_TYPE_OPENCODE_GO: 'OpenCode Go',
}

export const providerOptions = Object.entries(providerLabels).map(([value, label]) => ({
  value: value as ProviderType,
  label,
}))

export function getSection(id: Section): SectionDefinition {
  return sections.find((section) => section.id === id) ?? sections[0]
}
