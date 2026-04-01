/**
 * Project Card Header Slots
 *
 * These components are slotted into the shell's CardHeader.
 */

import React, { useRef, useEffect, useState } from 'react';
import { Input } from '@/components/ui/input';
import { ProjectTypeIcon } from '../components/CanvasCard/ProjectTypeIcon';
import { ProjectTypeBadge } from '../components/ui/ProjectTypeBadge';
import { getProjectDisplayName, hasProjectName } from '../utils/projectName';
import { getPlugin } from '../project-types';
import { useStore } from '../store/useStore';
import { api } from '../api/client';
import type { Project } from '../types';

interface ProjectTitleSlotProps {
  project: Project;
  allProjects: Project[];
}

/**
 * Title slot content for project cards
 * Renders: Icon, Badge (if applicable), Project Name (editable), Auto-naming indicator
 */
export const ProjectTitleSlot: React.FC<ProjectTitleSlotProps> = ({ project, allProjects }) => {
  const nameInputRef = useRef<HTMLInputElement>(null);
  const [isEditing, setIsEditing] = useState(false);
  const [editedName, setEditedName] = useState(project.name);
  const updateProject = useStore((s) => s.updateProject);
  const bringToFront = useStore((s) => s.bringCardToFront);

  useEffect(() => {
    if (isEditing && nameInputRef.current) {
      nameInputRef.current.focus();
    }
  }, [isEditing]);

  const handleRename = async () => {
    if (!editedName.trim() || editedName === project.name) {
      setEditedName(project.name);
      setIsEditing(false);
      return;
    }
    try {
      await api.patch(`/api/projects/${project.id}`, { name: editedName.trim() });
      updateProject({ ...project, name: editedName.trim() });
    } catch (error) {
      // Silently revert on failure - user will see name didn't change
      setEditedName(project.name);
    } finally {
      setIsEditing(false);
    }
  };

  const handleNameClick = (e: React.MouseEvent) => {
    e.stopPropagation();
    bringToFront(project.id);
  };

  const handleNameDoubleClick = (e: React.MouseEvent) => {
    e.stopPropagation();
    setIsEditing(true);
  };

  const handleKeyDown = (e: React.KeyboardEvent) => {
    if (e.key === 'Enter') {
      handleRename();
    }
    if (e.key === 'Escape') {
      setEditedName(project.name);
      setIsEditing(false);
    }
  };

  if (isEditing) {
    return (
      <Input
        ref={nameInputRef}
        type="text"
        value={editedName}
        onChange={(e) => setEditedName(e.target.value)}
        onBlur={handleRename}
        onKeyDown={handleKeyDown}
        onClick={(e) => e.stopPropagation()}
        style={{
          transition: 'all var(--animation-responsive) var(--ease-smooth)',
        }}
        className="flex-1 font-semibold text-foreground bg-background border border-primary rounded px-2 py-1 text-sm
          focus:outline-none focus:ring-2 focus:ring-primary focus:shadow-[0_0_0_4px_hsl(var(--primary)/0.1)] min-w-0"
      />
    );
  }

  return (
    <>
      <ProjectTypeIcon agentType={project.agentType || 'claude-code-chat'} />
      {/* Show badge only if it differs from project name */}
      {(() => {
        const plugin = getPlugin(project.agentType || 'claude-code-chat');
        const badgeText = plugin?.badgeText;
        const shouldShowBadge = badgeText && badgeText !== project.name;
        return shouldShowBadge ? (
          <ProjectTypeBadge projectType={project.agentType || 'claude-code-chat'} />
        ) : null;
      })()}
      {/* eslint-disable-next-line jsx-a11y/no-static-element-interactions, jsx-a11y/click-events-have-key-events -- Project name is not a primary interactive element; double-click to edit is secondary action; primary interaction is dragging the card */}
      <h3
        className={`font-semibold truncate cursor-pointer hover:text-primary transition-colors min-w-0 flex-shrink text-sm ${
          hasProjectName(project) ? 'text-foreground' : 'text-muted-foreground italic'
        }`}
        onClick={handleNameClick}
        onDoubleClick={handleNameDoubleClick}
        title="Double-click to rename"
      >
        {getProjectDisplayName(project, allProjects)}
      </h3>
    </>
  );
};
