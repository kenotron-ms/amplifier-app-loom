/**
 * Settings module - public exports
 *
 * Usage:
 *   import { useTheme, useSettingsStore } from '../settings';
 */

// Types
export type { Theme, CachedSettings } from './constants';

// Store and hooks
export { useSettingsStore, useTheme, useSettingsSynced } from './useSettingsStore';

// Cache (for advanced use cases only)
export { settingsCache } from './settingsCache';
