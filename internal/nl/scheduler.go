package nl

import "github.com/ms/amplifier-app-loom/internal/types"

// JobScheduler is the minimal interface the NL tool executor needs to keep the
// live cron scheduler in sync with job changes.  The concrete
// *scheduler.Scheduler satisfies this interface; the interface lives here to
// avoid a circular import between the nl and scheduler packages.
type JobScheduler interface {
	RemoveJob(jobID string)
	AddJob(job *types.Job)
}
