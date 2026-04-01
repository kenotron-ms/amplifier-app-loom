/**
 * Settings cache - localStorage abstraction layer
 *
 * This module encapsulates all localStorage access for settings.
 * Components should never access localStorage directly for settings.
 *
 * Design principles:
 * - Synchronous reads (cache is always available)
 * - localStorage is implementation detail (could swap to IndexedDB)
 * - Type-safe with validation
 * - Graceful degradation if localStorage unavailable
 */

import {
  SETTINGS_STORAGE_KEYS,
  SETTINGS_DEFAULTS,
  VALID_THEMES,
  type Theme,
  type CachedSettings,
  type ApiKeyMetadata,
} from './constants';
import { createLogger } from '@workspaces/tracing';

const logger = createLogger({ component: 'SettingsCache' });

function isValidTheme(value: unknown): value is Theme {
  return typeof value === 'string' && VALID_THEMES.includes(value as Theme);
}

function isValidApiKeyMetadata(value: unknown): value is ApiKeyMetadata {
  if (!value || typeof value !== 'object') return false;
  const obj = value as Record<string, unknown>;
  return (
    typeof obj.hasKey === 'boolean' &&
    (obj.createdAt === undefined || typeof obj.createdAt === 'number')
  );
}

/**
 * Safe localStorage wrapper
 */
function safeGet(key: string): string | null {
  try {
    return localStorage.getItem(key);
  } catch {
    return null;
  }
}

function safeSet(key: string, value: string): void {
  try {
    localStorage.setItem(key, value);
  } catch (error) {
    logger.warn(`Failed to write ${key}`, error as Error);
  }
}

function safeRemove(key: string): void {
  try {
    localStorage.removeItem(key);
  } catch {
    // Ignore errors on remove
  }
}

/**
 * Settings cache API
 */
export const settingsCache = {
  /**
   * Get cached theme (synchronous, always returns valid value)
   */
  getTheme(): Theme {
    const stored = safeGet(SETTINGS_STORAGE_KEYS.theme);
    return isValidTheme(stored) ? stored : SETTINGS_DEFAULTS.theme;
  },

  /**
   * Set cached theme
   */
  setTheme(theme: Theme): void {
    if (!isValidTheme(theme)) {
      logger.warn(`Invalid theme: ${theme}`);
      return;
    }
    safeSet(SETTINGS_STORAGE_KEYS.theme, theme);
  },

  /**
   * Get cached API key metadata (synchronous, returns null if not found)
   */
  getApiKeyMetadata(): ApiKeyMetadata | null {
    const stored = safeGet(SETTINGS_STORAGE_KEYS.apiKeyMetadata);
    if (!stored) return null;

    try {
      const parsed = JSON.parse(stored);
      return isValidApiKeyMetadata(parsed) ? parsed : null;
    } catch {
      return null;
    }
  },

  /**
   * Set cached API key metadata
   */
  setApiKeyMetadata(metadata: ApiKeyMetadata | null): void {
    if (metadata === null) {
      safeRemove(SETTINGS_STORAGE_KEYS.apiKeyMetadata);
      return;
    }

    if (!isValidApiKeyMetadata(metadata)) {
      logger.warn('Invalid API key metadata', { metadata });
      return;
    }

    safeSet(SETTINGS_STORAGE_KEYS.apiKeyMetadata, JSON.stringify(metadata));
  },

  /**
   * Get cached auto-switch to preview setting (synchronous)
   */
  getAutoSwitchToPreview(): boolean {
    const stored = safeGet(SETTINGS_STORAGE_KEYS.autoSwitchToPreview);
    if (stored === 'true') return true;
    if (stored === 'false') return false;
    return SETTINGS_DEFAULTS.autoSwitchToPreview;
  },

  /**
   * Set cached auto-switch to preview setting
   */
  setAutoSwitchToPreview(enabled: boolean): void {
    safeSet(SETTINGS_STORAGE_KEYS.autoSwitchToPreview, String(enabled));
  },

  /**
   * Get cached enable terminal agents setting (synchronous)
   * Returns null if not set (use env default), true/false if user chose
   */
  getEnableTerminalAgents(): boolean | null {
    const stored = safeGet(SETTINGS_STORAGE_KEYS.enableTerminalAgents);
    if (stored === 'true') return true;
    if (stored === 'false') return false;
    return null; // Not set - use env default
  },

  /**
   * Set cached enable terminal agents setting
   */
  setEnableTerminalAgents(enabled: boolean | null): void {
    if (enabled === null) {
      safeRemove(SETTINGS_STORAGE_KEYS.enableTerminalAgents);
    } else {
      safeSet(SETTINGS_STORAGE_KEYS.enableTerminalAgents, String(enabled));
    }
  },

  /**
   * Get all cached settings (for initializing stores)
   */
  getAll(): CachedSettings {
    return {
      theme: this.getTheme(),
      apiKeyMetadata: this.getApiKeyMetadata(),
      autoSwitchToPreview: this.getAutoSwitchToPreview(),
      enableTerminalAgents: this.getEnableTerminalAgents(),
    };
  },

  /**
   * Update multiple settings at once
   */
  setAll(settings: Partial<CachedSettings>): void {
    if (settings.theme !== undefined) {
      this.setTheme(settings.theme);
    }
    if (settings.apiKeyMetadata !== undefined) {
      this.setApiKeyMetadata(settings.apiKeyMetadata);
    }
    if (settings.autoSwitchToPreview !== undefined) {
      this.setAutoSwitchToPreview(settings.autoSwitchToPreview);
    }
    if (settings.enableTerminalAgents !== undefined) {
      this.setEnableTerminalAgents(settings.enableTerminalAgents);
    }
  },

  /**
   * Clear all cached settings (for logout)
   */
  clear(): void {
    safeRemove(SETTINGS_STORAGE_KEYS.theme);
    safeRemove(SETTINGS_STORAGE_KEYS.apiKeyMetadata);
    safeRemove(SETTINGS_STORAGE_KEYS.autoSwitchToPreview);
    safeRemove(SETTINGS_STORAGE_KEYS.enableTerminalAgents);
  },
};
