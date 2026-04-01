/**
 * Project Card Plugin - Adapter Layer
 *
 * Bridges shell's CardPlugin with app's ProjectTypePlugin.
 * This is the single plugin registered with the shell, which internally
 * delegates to the appropriate project-type plugin.
 */

import type { CardPlugin, CardBodyProps, CardType } from './shell-types';
import { useStore } from '../store/useStore';
import { api } from '../api/client';
import { CanvasCard } from '../components/CanvasCard';
import { ViewModeToggle } from '../components/CanvasCard/ViewModeToggle';
import { ProjectTitleSlot } from './ProjectCardHeaderSlots';
import { getArchetype } from '../project-types';

interface ProjectPluginData {
  projectId: string;
}

/**
 * Card Body Component - renders just the body (no header)
 */
function ProjectCardBody(props: CardBodyProps<ProjectPluginData>) {
  const projects = useStore((s) => s.projects || []);
  const project = projects.find((p) => p.id === props.pluginData.projectId);

  if (!project) {
    return <div className="p-4 text-muted-foreground">Project not found</div>;
  }

  // Render just the body - header is provided via slots
  return <CanvasCard project={project} />;
}

/**
 * Header Title Component - uses hooks properly
 */
function ProjectHeaderTitle({ pluginData }: { pluginData: ProjectPluginData }) {
  const projects = useStore((s) => s.projects || []);
  const project = projects.find((p) => p.id === pluginData.projectId);

  if (!project) return null;

  return <ProjectTitleSlot project={project} allProjects={projects} />;
}

/**
 * Header Actions Component - renders ViewModeToggle for chat-archetype projects
 *
 * Hidden for terminal-mode projects (they have no preview or file browsing).
 */
function ProjectHeaderActions({ pluginData }: { pluginData: ProjectPluginData }) {
  const projects = useStore((s) => s.projects || []);
  const project = projects.find((p) => p.id === pluginData.projectId);
  const viewMode = useStore((s) => s.getProjectViewMode(pluginData.projectId));
  const setProjectViewMode = useStore((s) => s.setProjectViewMode);

  if (!project) return null;

  // Toggle is only shown for chat archetype; terminal projects don't support preview/files
  const archetype = getArchetype(project.agentType || 'claude-code-chat');
  if (archetype === 'terminal') return null;

  return (
    <ViewModeToggle
      viewMode={viewMode}
      onViewModeChange={(mode) => setProjectViewMode(pluginData.projectId, mode)}
    />
  );
}

/**
 * Create the project card adapter plugin
 */
export function createProjectCardPlugin(): CardPlugin<ProjectPluginData> {
  return {
    id: 'project-card',
    name: 'Project Card',

    // Use slots for header customization
    renderHeaderTitle: ({ pluginData }) => {
      const projects = useStore.getState().projects || [];
      const project = projects.find((p) => p.id === pluginData.projectId);
      if (!project) return null;
      return <ProjectHeaderTitle pluginData={pluginData} />;
    },

    // View mode toggle (Preview / Chat / Files) — hidden for terminal projects
    renderHeaderActions: ({ pluginData }) => {
      return <ProjectHeaderActions pluginData={pluginData} />;
    },

    // CanvasCard only renders body (header via slots)
    skipContentPadding: true,

    renderBody: (props: CardBodyProps<ProjectPluginData>) => {
      return <ProjectCardBody {...props} />;
    },

    lifecycle: {
      onCardMinimized: async (card: CardType) => {
        const projectId = (card.pluginData as ProjectPluginData).projectId;
        useStore.getState().minimizeProject(projectId);
      },

      onCardRestored: async (card: CardType) => {
        const projectId = (card.pluginData as ProjectPluginData).projectId;
        useStore.getState().restoreProject(projectId);
      },

      onCardFullscreenEnter: async (card: CardType) => {
        const projectId = (card.pluginData as ProjectPluginData).projectId;
        const project = useStore.getState().projects.find((p) => p.id === projectId);
        if (project) {
          useStore.getState().setActiveProject(project);
          useStore.getState().setView('fullscreen');
        }
      },

      onCardFullscreenExit: async (_card: CardType) => {
        // When exiting fullscreen, return to workspace view
        useStore.getState().setView('workspace');
      },

      onCardDestroyed: async (card: CardType) => {
        const projectId = (card.pluginData as ProjectPluginData).projectId;
        await api.delete(`/api/projects/${projectId}`);
        useStore.getState().deleteProject(projectId);
      },
    },
  };
}
