/**
 * Settings cache constants
 *
 * SYNC:index.html - These values are duplicated in the inline script
 * in index.html for flash prevention. If you change these values,
 * update the inline script too!
 */

export const SETTINGS_STORAGE_KEYS = {
  theme: 'canvas:settings:theme',
  apiKeyMetadata: 'canvas:settings:apiKeyMetadata',
  autoSwitchToPreview: 'canvas:settings:autoSwitchToPreview',
  enableTerminalAgents: 'canvas:settings:enableTerminalAgents',
  // Future settings can be added here:
  // locale: 'canvas:settings:locale',
  // sidebarCollapsed: 'canvas:settings:sidebarCollapsed',
} as const;

export const SETTINGS_DEFAULTS = {
  theme: 'dark' as const,
  apiKeyMetadata: null,
  autoSwitchToPreview: false,
  enableTerminalAgents: null, // null = use env default, let user choose to override
  // Future defaults:
  // locale: 'en',
  // sidebarCollapsed: false,
};

export const VALID_THEMES = ['dark', 'light', 'warm'] as const;

export type Theme = (typeof VALID_THEMES)[number];

/**
 * API key reentry reasons - imported from shared types package
 */
import type { ApiKeyReentryReason } from '@workspaces/types';
export { API_KEY_REENTRY_REASONS } from '@workspaces/types';
export type { ApiKeyReentryReason };

export interface ApiKeyMetadata {
  hasKey: boolean;
  createdAt?: number;
  needsReentry?: boolean;
  reason?: ApiKeyReentryReason;
}

export interface CachedSettings {
  theme: Theme;
  apiKeyMetadata: ApiKeyMetadata | null;
  autoSwitchToPreview: boolean;
  enableTerminalAgents: boolean | null; // null = use env default, true/false = user choice
  // Extensible: add new settings here
}
