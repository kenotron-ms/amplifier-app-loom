/**
 * Settings store - React integration for cached settings
 *
 * Responsibilities:
 * - Initialize from cache (synchronous, no flash)
 * - Sync from database after authentication
 * - Provide reactive state to components
 * - Write-through to cache on changes
 */

import { create } from 'zustand';
import { settingsCache } from './settingsCache';
import { settingsApi } from '../api/settingsApi';
import type { Theme, CachedSettings, ApiKeyMetadata } from './constants';
import { createLogger } from '@workspaces/tracing';

const logger = createLogger({ component: 'SettingsStore' });

interface SettingsState extends CachedSettings {
  // Sync state
  isSynced: boolean;
  isSyncing: boolean;
  syncError: string | null;

  // Actions
  syncFromDatabase: () => Promise<void>;
  setTheme: (theme: Theme) => Promise<void>;
  setApiKey: (key: string) => Promise<void>;
  removeApiKey: () => Promise<void>;
  setAutoSwitchToPreview: (enabled: boolean) => Promise<void>;
  setEnableTerminalAgents: (enabled: boolean) => Promise<void>;
  reset: () => void;
}

/**
 * Apply theme to DOM
 */
function applyThemeToDOM(theme: Theme): void {
  document.documentElement.setAttribute('data-theme', theme);
}

// Environment default from build-time constant
const ENV_DEFAULT_TERMINAL = import.meta.env.VITE_ENABLE_TERMINAL_AGENTS === 'true';

export const useSettingsStore = create<SettingsState>((set, get) => ({
  // Initialize from cache (synchronous - no flash)
  ...settingsCache.getAll(),

  // Sync state
  isSynced: false,
  isSyncing: false,
  syncError: null,

  /**
   * Sync settings from database
   * Called after authentication is confirmed
   */
  syncFromDatabase: async () => {
    // Prevent duplicate syncs
    if (get().isSyncing || get().isSynced) return;

    set({ isSyncing: true, syncError: null });

    try {
      // Fetch all settings in parallel
      const [themeData, apiKeyData, autoPreviewData, enableTerminalData] = await Promise.all([
        settingsApi.getTheme(),
        settingsApi.getApiKeyStatus(),
        settingsApi.getAutoPreview(),
        settingsApi.getEnableTerminalAgents(),
      ]);

      const settings = {
        theme: themeData.theme,
        apiKeyMetadata: apiKeyData.hasKey
          ? { hasKey: true, createdAt: apiKeyData.createdAt }
          : apiKeyData.needsReentry
            ? {
                hasKey: false,
                needsReentry: true,
                reason: apiKeyData.reason,
              }
            : null,
        autoSwitchToPreview: autoPreviewData.autoSwitchToPreview,
        // null means user hasn't chosen, true/false means explicit choice
        enableTerminalAgents: enableTerminalData.enableTerminalAgents,
      };

      // Update cache
      settingsCache.setAll(settings);

      // Update store and DOM
      set({ ...settings, isSynced: true, isSyncing: false });
      applyThemeToDOM(settings.theme);
    } catch (error) {
      logger.error('Sync failed', error as Error);
      set({
        syncError: error instanceof Error ? error.message : 'Sync failed',
        isSyncing: false,
        // Keep isSynced false so retry is possible
      });
      // Cache value is still applied (graceful degradation)
    }
  },

  /**
   * Update theme (writes to DB, cache, and DOM)
   */
  setTheme: async (theme: Theme) => {
    const previousTheme = get().theme;

    // Optimistic update: DOM + store immediately
    applyThemeToDOM(theme);
    set({ theme });

    // Write-through to cache
    settingsCache.setTheme(theme);

    // Persist to database
    try {
      await settingsApi.updateTheme(theme);
    } catch (error) {
      logger.error('Failed to save theme', error as Error);
      // Revert on failure
      applyThemeToDOM(previousTheme);
      set({ theme: previousTheme });
      settingsCache.setTheme(previousTheme);
      throw error;
    }
  },

  /**
   * Set API key (validates and stores encrypted in DB)
   */
  setApiKey: async (key: string) => {
    try {
      const result = await settingsApi.updateApiKey(key);
      const metadata: ApiKeyMetadata = {
        hasKey: true,
        createdAt: result.updatedAt,
      };

      // Update cache and store
      settingsCache.setApiKeyMetadata(metadata);
      set({ apiKeyMetadata: metadata });
    } catch (error) {
      logger.error('Failed to save API key', error as Error);
      throw error;
    }
  },

  /**
   * Remove API key
   */
  removeApiKey: async () => {
    try {
      await settingsApi.removeApiKey();

      // Update cache and store
      settingsCache.setApiKeyMetadata(null);
      set({ apiKeyMetadata: null });
    } catch (error) {
      logger.error('Failed to remove API key', error as Error);
      throw error;
    }
  },

  /**
   * Update auto-switch to preview setting
   */
  setAutoSwitchToPreview: async (enabled: boolean) => {
    const previous = get().autoSwitchToPreview;

    // Optimistic update
    set({ autoSwitchToPreview: enabled });

    // Write-through to cache
    settingsCache.setAutoSwitchToPreview(enabled);

    // Persist to database
    try {
      await settingsApi.updateAutoPreview(enabled);
    } catch (error) {
      logger.error('Failed to save auto-preview setting', error as Error);
      // Revert on failure
      set({ autoSwitchToPreview: previous });
      settingsCache.setAutoSwitchToPreview(previous);
      throw error;
    }
  },

  /**
   * Update enable terminal agents setting
   */
  setEnableTerminalAgents: async (enabled: boolean | null) => {
    const previous = get().enableTerminalAgents;

    // Optimistic update
    set({ enableTerminalAgents: enabled });

    // Write-through to cache
    settingsCache.setEnableTerminalAgents(enabled);

    // Persist to database
    try {
      await settingsApi.updateEnableTerminalAgents(enabled);
    } catch (error) {
      logger.error('Failed to save enable-terminal-agents setting', error as Error);
      // Revert on failure
      set({ enableTerminalAgents: previous });
      settingsCache.setEnableTerminalAgents(previous);
      throw error;
    }
  },

  /**
   * Reset to defaults (for logout)
   */
  reset: () => {
    settingsCache.clear();
    const defaults = settingsCache.getAll();
    set({
      ...defaults,
      isSynced: false,
      isSyncing: false,
      syncError: null,
    });
    applyThemeToDOM(defaults.theme);
  },
}));

// Convenience hooks for components
export const useTheme = () => useSettingsStore((state) => state.theme);
export const useApiKeyMetadata = () => useSettingsStore((state) => state.apiKeyMetadata);
export const useSettingsSynced = () => useSettingsStore((state) => state.isSynced);
export const useAutoSwitchToPreview = () => useSettingsStore((state) => state.autoSwitchToPreview);

// Returns EFFECTIVE value (user preference OR env default)
export const useEnableTerminalAgents = () => {
  const pref = useSettingsStore((state) => state.enableTerminalAgents);
  return pref !== null ? pref : ENV_DEFAULT_TERMINAL;
};
