import { memo, useState, useEffect } from 'react';
import { Button } from '@/components/ui/button';
import { ProjectTypeBadge } from '../components/ui/ProjectTypeBadge';
import { getPlugin } from '../project-types';
import { useStore } from '../store/useStore';
import type { CardType } from './shell-types';

export interface ProjectDockItemProps {
  card: CardType;
  onClick: () => void;
  index?: number;
}

/**
 * ProjectDockItem - Dock item with full project integration
 * Matches the styling and functionality of the old workspace's DockItem
 */
export const ProjectDockItem = memo(function ProjectDockItem({
  card,
  onClick,
  index = 0,
}: ProjectDockItemProps) {
  const [isHovered, setIsHovered] = useState(false);
  const [elapsedSeconds, setElapsedSeconds] = useState(0);

  // Get project data from pluginData
  const pluginData = card.pluginData as { projectId?: string } | undefined;
  const projectId = pluginData?.projectId;

  // Get project from store
  const project = useStore((state) => state.projects.find((p) => p.id === projectId));

  // Get execution state
  const isExecuting = useStore((state) =>
    projectId ? state.isProjectExecuting(projectId) : false
  );
  const executionStartTime = useStore((state) =>
    projectId ? state.getExecutionStartTime(projectId) : null
  );

  // Update elapsed time when executing
  useEffect(() => {
    if (!isExecuting || !executionStartTime) {
      setElapsedSeconds(0);
      return;
    }

    setElapsedSeconds(Math.floor((Date.now() - executionStartTime) / 1000));

    const interval = setInterval(() => {
      setElapsedSeconds(Math.floor((Date.now() - executionStartTime) / 1000));
    }, 1000);

    return () => clearInterval(interval);
  }, [isExecuting, executionStartTime]);

  // Format elapsed time
  const formatElapsedTime = (seconds: number): string => {
    if (seconds < 60) return `${seconds}s`;
    const mins = Math.floor(seconds / 60);
    const secs = seconds % 60;
    return `${mins}m ${secs}s`;
  };

  // Format relative time
  const getRelativeTime = (timestamp: number) => {
    const now = Date.now();
    const diff = now - timestamp;
    const minutes = Math.floor(diff / 60000);
    const hours = Math.floor(diff / 3600000);
    const days = Math.floor(diff / 86400000);

    if (minutes < 1) return 'Just now';
    if (minutes < 60) return `${minutes} ${minutes === 1 ? 'minute' : 'minutes'} ago`;
    if (hours < 24) return `${hours} ${hours === 1 ? 'hour' : 'hours'} ago`;
    if (days < 7) return `${days} ${days === 1 ? 'day' : 'days'} ago`;

    const date = new Date(timestamp);
    const currentYear = new Date().getFullYear();
    const timestampYear = date.getFullYear();

    if (timestampYear === currentYear) {
      return date.toLocaleDateString('en-US', { month: 'short', day: 'numeric' });
    }
    return date.toLocaleDateString('en-US', { month: 'short', day: 'numeric', year: 'numeric' });
  };

  // Get project icon
  const getProjectTypeIcon = () => {
    const plugin = getPlugin(project?.agentType ?? 'claude-code-chat');
    const Icon = plugin?.icon;

    if (!Icon) {
      return (
        <svg className="w-4 h-4 flex-shrink-0" viewBox="0 0 24 24" fill="currentColor">
          <circle cx="12" cy="12" r="10" />
        </svg>
      );
    }

    return <Icon className="w-4 h-4 flex-shrink-0" />;
  };

  // Get status indicator
  const getStatusIndicator = () => {
    if (isExecuting) {
      return (
        <span className="w-1.5 h-1.5 rounded-full animate-pulse bg-primary" aria-hidden="true" />
      );
    }
    return null;
  };

  if (!project) {
    // Fallback if project not found
    return (
      <Button onClick={onClick} variant="ghost" className="w-full text-left rounded-xl">
        <span className="text-sm">{card.title}</span>
      </Button>
    );
  }

  return (
    <Button
      onClick={onClick}
      onMouseEnter={() => setIsHovered(true)}
      onMouseLeave={() => setIsHovered(false)}
      variant="ghost"
      aria-label={`Restore ${card.title}`}
      data-testid="dock-item"
      className="w-full text-left rounded-xl transition-all duration-300 group relative overflow-hidden h-auto justify-start"
      style={{
        minHeight: '44px',
        padding: '10px 12px',
        background: isHovered ? 'hsl(var(--card))' : 'hsl(var(--background))',
        border: `1px solid ${isHovered ? 'hsl(var(--primary))' : 'hsl(var(--border))'}`,
        boxShadow: isHovered
          ? '0 4px 12px hsl(var(--foreground) / 0.15)'
          : '0 1px 3px hsl(var(--foreground) / 0.1)',
        transform: isHovered ? 'translateX(-2px)' : 'translateX(0)',
        transitionTimingFunction: 'cubic-bezier(0.34, 1.56, 0.64, 1)',
        animation: `slideIn 300ms cubic-bezier(0.34, 1.56, 0.64, 1) ${index * 50}ms both`,
      }}
    >
      {isHovered && (
        <div
          className="absolute inset-0 pointer-events-none"
          style={{
            background:
              'radial-gradient(circle at 0% 50%, hsl(var(--primary) / 0.1) 0%, transparent 70%)',
          }}
        />
      )}

      <div className="flex flex-col gap-0.5 relative">
        <div className="flex items-center gap-2">
          <span
            className="flex-shrink-0 transition-opacity duration-300"
            style={{
              opacity: isHovered ? 1 : 0.7,
            }}
          >
            {getProjectTypeIcon()}
          </span>

          <ProjectTypeBadge projectType={project.agentType ?? 'claude-code-chat'} />

          <span
            className="flex-1 text-sm font-medium truncate transition-colors duration-300"
            style={{
              color: isHovered ? 'hsl(var(--foreground))' : 'hsl(var(--muted-foreground))',
            }}
          >
            {card.title}
          </span>

          <span
            className="text-xs text-primary transition-all duration-300"
            style={{
              opacity: isHovered ? 1 : 0,
              transform: isHovered ? 'translateX(0)' : 'translateX(4px)',
            }}
            aria-hidden="true"
          >
            Restore
          </span>
        </div>

        <div className="flex items-center gap-1.5 pl-5">
          {getStatusIndicator()}

          <span
            className="transition-colors duration-300"
            style={{
              color: isExecuting ? 'hsl(var(--success))' : 'hsl(var(--muted-foreground))',
              fontSize: '10px',
            }}
          >
            {isExecuting
              ? `Working... ${formatElapsedTime(elapsedSeconds)}`
              : project.lastActivityAt
                ? getRelativeTime(project.lastActivityAt)
                : getRelativeTime(project.createdAt)}
          </span>
        </div>
      </div>

      <style>{`
        @keyframes slideIn {
          from {
            opacity: 0;
            transform: translateX(20px);
          }
          to {
            opacity: 1;
            transform: translateX(0);
          }
        }
      `}</style>
    </Button>
  );
});
