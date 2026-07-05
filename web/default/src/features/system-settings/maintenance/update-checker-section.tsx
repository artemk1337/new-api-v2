/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
import { useEffect, useState } from 'react'
import { DownloadIcon, ExternalLinkIcon, RefreshCcwIcon } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { getStatus } from '@/lib/api'
import { formatTimestamp, formatTimestampToDate } from '@/lib/format'
import { cn } from '@/lib/utils'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Markdown } from '@/components/ui/markdown'
import { Dialog } from '@/components/dialog'
import {
  checkSystemUpdate,
  getCurrentSystemUpdateTask,
  getSystemTask,
  getSystemUpdateJob,
  listSystemTasks,
  startSystemUpdate,
} from '../api'
import { SettingsSection } from '../components/settings-section'
import type { SystemUpdateRelease, SystemUpdateTask } from '../types'
import { shouldResumeDeployingSystemUpdateTask } from './system-update-resume'

type UpdateCheckerSectionProps = {
  currentVersion?: string | null
  startTime?: number | null
}

type UpdateStepState = 'idle' | 'running' | 'done' | 'error'

type UpdateStep = {
  step: string
}

function isActiveSystemUpdateTask(task: SystemUpdateTask | null) {
  return task?.status === 'pending' || task?.status === 'running'
}

const updateVersionWaitTimeoutMs = 10 * 60 * 1000
const updateJobLookupFailureLimit = 5
const signInRedirectPath = '/sign-in?redirect=%2Fdashboard'
const systemUpdateSteps: UpdateStep[] = [
  { step: 'checking' },
  { step: 'requesting_updater' },
  { step: 'pulling' },
  { step: 'deploying' },
  { step: 'succeeded' },
]

function getSystemUpdateStepIndex(step: string | undefined) {
  const index = systemUpdateSteps.findIndex((item) => item.step === step)
  return index >= 0 ? index : 0
}

function getSystemUpdateStepState(
  task: SystemUpdateTask,
  index: number
): UpdateStepState {
  if (task.status === 'failed') {
    const failedIndex = getSystemUpdateStepIndex(task.state?.step)
    if (index < failedIndex) return 'done'
    if (index === failedIndex) return 'error'
    return 'idle'
  }
  if (task.status === 'succeeded' && task.state?.step === 'succeeded') {
    return 'done'
  }

  const currentIndex = getSystemUpdateStepIndex(task.state?.step)
  if (index < currentIndex) return 'done'
  if (index === currentIndex) return 'running'
  return 'idle'
}

function getSystemUpdateStepLabel(t: (key: string) => string, step: string) {
  switch (step) {
    case 'checking':
      return t('Validate update tag')
    case 'requesting_updater':
      return t('Request updater sidecar')
    case 'pulling':
      return t('Pull update image')
    case 'deploying':
      return t('Deploy service')
    case 'succeeded':
      return t('Confirm new version')
    default:
      return step
  }
}

export function UpdateCheckerSection({
  currentVersion,
  startTime,
}: UpdateCheckerSectionProps) {
  const { t } = useTranslation()
  const [checking, setChecking] = useState(false)
  const [updating, setUpdating] = useState(false)
  const [dialogOpen, setDialogOpen] = useState(false)
  const [release, setRelease] = useState<SystemUpdateRelease | null>(null)
  const [releases, setReleases] = useState<SystemUpdateRelease[]>([])
  const [canUpdate, setCanUpdate] = useState(false)
  const [updateTask, setUpdateTask] = useState<SystemUpdateTask | null>(null)
  const [expectedUpdateVersion, setExpectedUpdateVersion] = useState<
    string | null
  >(null)
  const [expectedUpdateStartedAt, setExpectedUpdateStartedAt] = useState<
    number | null
  >(null)
  const [updateTaskId, setUpdateTaskId] = useState<string | null>(null)
  const [updateJobId, setUpdateJobId] = useState<string | null>(null)
  const [updateJobLookupFailures, setUpdateJobLookupFailures] = useState(0)
  const [targetVersion, setTargetVersion] = useState('')

  const uptime = startTime ? formatTimestamp(startTime) : t('Unknown')
  const version = currentVersion || t('Unknown')
  const updateActive = isActiveSystemUpdateTask(updateTask)
  const showUpdateTask = Boolean(updateTask && expectedUpdateVersion)

  const handleCheckUpdates = async () => {
    setChecking(true)
    try {
      const res = await checkSystemUpdate()
      if (!res.success || !res.data) {
        throw new Error(res.message || t('Failed to check for updates'))
      }
      const data = res.data.release
      if (!data?.tag_name) {
        toast.success(
          t('You are running the latest version ({{version}}).', {
            version: res.data.current_version,
          })
        )
        return
      }

      if (!res.data.update_available) {
        toast.success(
          t('You are running the latest version ({{version}}).', {
            version: data.tag_name,
          })
        )
        return
      }

      setRelease(data)
      setReleases(res.data.releases ?? [data])
      setCanUpdate(res.data.can_update)
      setDialogOpen(true)
    } catch (error) {
      const message =
        error instanceof Error
          ? error.message
          : t('Failed to check for updates')
      toast.error(message)
    } finally {
      setChecking(false)
    }
  }

  const startUpdateForVersion = async (version: string) => {
    const requestedVersion = version.trim()
    if (!requestedVersion) {
      toast.error(t('Version tag is required.'))
      return
    }

    setUpdating(true)
    try {
      const res = await startSystemUpdate(requestedVersion)
      if (!res.success || !res.data) {
        throw new Error(res.message || t('Failed to start system update'))
      }
      setUpdateTask(res.data)
      setUpdateTaskId(res.data.task_id)
      setUpdateJobId(null)
      setUpdateJobLookupFailures(0)
      setExpectedUpdateVersion(requestedVersion)
      setExpectedUpdateStartedAt(Date.now())
      toast.success(
        t('Installing {{version}}. The service may restart soon.', {
          version: requestedVersion,
        })
      )
    } catch (error) {
      const message =
        error instanceof Error
          ? error.message
          : t('Failed to start system update')
      toast.error(message)
    } finally {
      setUpdating(false)
    }
  }

  const handleStartUpdate = async () => {
    if (!release?.tag_name) return
    await startUpdateForVersion(release.tag_name)
  }

  const handleStartTargetVersion = async () => {
    await startUpdateForVersion(targetVersion)
  }

  const goToRelease = () => {
    if (release?.html_url) {
      window.open(release.html_url, '_blank', 'noopener,noreferrer')
    }
  }

  useEffect(() => {
    async function fetchCurrentSystemUpdateTask() {
      try {
        const res = await getCurrentSystemUpdateTask()
        if (res.success && res.data) {
          setUpdateTask(res.data)
          setUpdateTaskId(res.data.task_id)
          setExpectedUpdateVersion(res.data.payload?.version ?? null)
          setExpectedUpdateStartedAt(Date.now())
          return
        }

        const listRes = await listSystemTasks(10)
        if (listRes.success && listRes.data) {
          const deployingTask = listRes.data.find((task) =>
            shouldResumeDeployingSystemUpdateTask(
              task as SystemUpdateTask,
              currentVersion
            )
          ) as SystemUpdateTask | undefined
          if (deployingTask) {
            setUpdateTask(deployingTask)
            setUpdateTaskId(deployingTask.task_id)
            setUpdateJobId(deployingTask.result?.job_id ?? null)
            setUpdateJobLookupFailures(0)
            setExpectedUpdateVersion(
              deployingTask.result?.requested_version ?? null
            )
            setExpectedUpdateStartedAt(Date.now())
          }
        }
      } catch {
        // The service may be restarting during self-update; the next poll will retry.
      }
    }

    fetchCurrentSystemUpdateTask()
  }, [currentVersion])

  useEffect(() => {
    if (
      !expectedUpdateVersion ||
      currentVersion?.trim() !== expectedUpdateVersion
    ) {
      return
    }

    setExpectedUpdateVersion(null)
    setExpectedUpdateStartedAt(null)
    setUpdateTaskId(null)
    setUpdateJobId(null)
    setUpdateJobLookupFailures(0)
    setUpdateTask(null)
    window.setTimeout(() => window.location.assign(signInRedirectPath), 1200)
  }, [currentVersion, expectedUpdateVersion])

  useEffect(() => {
    if (!updateActive && !expectedUpdateVersion && !updateTaskId) return

    const interval = window.setInterval(async () => {
      try {
        const res = updateTaskId
          ? await getSystemTask<SystemUpdateTask>(updateTaskId)
          : await getCurrentSystemUpdateTask()
        if (res.success) {
          setUpdateTask(res.data ?? null)
          if (res.data?.status === 'failed') {
            toast.error(res.data.error || t('System update failed.'))
            setExpectedUpdateVersion(null)
            setExpectedUpdateStartedAt(null)
            setUpdateTaskId(null)
            setUpdateJobId(null)
            setUpdateJobLookupFailures(0)
            return
          }
          if (res.data?.result?.job_id) {
            setUpdateJobId(res.data.result.job_id)
          }
        }
      } catch {
        // The service may be restarting during self-update; keep polling.
      }

      if (updateJobId) {
        try {
          const res = await getSystemUpdateJob(updateJobId)
          setUpdateJobLookupFailures(0)
          if (res.success && res.data?.status === 'failed') {
            toast.error(res.data.error || t('System update failed and was rolled back.'))
            setExpectedUpdateVersion(null)
            setExpectedUpdateStartedAt(null)
            setUpdateTaskId(null)
            setUpdateJobId(null)
            setUpdateJobLookupFailures(0)
            return
          }
        } catch {
          const nextFailures = updateJobLookupFailures + 1
          if (nextFailures >= updateJobLookupFailureLimit) {
            toast.error(t('Could not read updater status.'))
            setExpectedUpdateVersion(null)
            setExpectedUpdateStartedAt(null)
            setUpdateTaskId(null)
            setUpdateJobId(null)
            setUpdateJobLookupFailures(0)
          } else {
            setUpdateJobLookupFailures(nextFailures)
          }
          return
        }
      }

      if (!expectedUpdateVersion) return

      try {
        const status = await getStatus()
        if (status?.version === expectedUpdateVersion) {
          toast.success(
            t('Updated to {{version}}. Reloading...', {
              version: expectedUpdateVersion,
            })
          )
          setExpectedUpdateVersion(null)
          setExpectedUpdateStartedAt(null)
          setUpdateTaskId(null)
          setUpdateJobId(null)
          setUpdateJobLookupFailures(0)
          setUpdateTask(null)
          window.setTimeout(
            () => window.location.assign(signInRedirectPath),
            1200
          )
        }
      } catch {
        // The service may be restarting during self-update; keep polling.
      }

      if (
        expectedUpdateStartedAt &&
        Date.now() - expectedUpdateStartedAt > updateVersionWaitTimeoutMs
      ) {
        toast.error(t('System update did not finish in time.'))
        setExpectedUpdateVersion(null)
        setExpectedUpdateStartedAt(null)
        setUpdateTaskId(null)
        setUpdateJobId(null)
        setUpdateJobLookupFailures(0)
      }
    }, 3000)

    return () => window.clearInterval(interval)
  }, [
    expectedUpdateStartedAt,
    expectedUpdateVersion,
    t,
    updateActive,
    updateJobId,
    updateJobLookupFailures,
    updateTaskId,
  ])

  return (
    <>
      <SettingsSection title={t('System maintenance')}>
        <div className='space-y-6'>
          <div className='grid gap-4 md:grid-cols-2'>
            <div className='rounded-lg border p-4'>
              <div className='text-muted-foreground text-sm'>
                {t('Current version')}
              </div>
              <div className='text-lg font-semibold'>{version}</div>
            </div>
            <div className='rounded-lg border p-4'>
              <div className='text-muted-foreground text-sm'>
                {t('Uptime since')}
              </div>
              <div className='text-lg font-semibold'>{uptime}</div>
            </div>
          </div>

          <Button onClick={handleCheckUpdates} disabled={checking}>
            {checking ? (
              t('Checking updates...')
            ) : (
              <>
                <RefreshCcwIcon className='me-2 h-4 w-4' />
                {t('Check for updates')}
              </>
            )}
          </Button>

          <div className='space-y-2'>
            <div>
              <div className='text-sm font-medium'>
                {t('Install or roll back to a specific tag')}
              </div>
              <div className='text-muted-foreground text-sm'>
                {t(
                  'Install any existing GitHub tag, including an older version for rollback.'
                )}
              </div>
            </div>
            <div className='flex flex-col gap-2 sm:flex-row'>
              <Input
                value={targetVersion}
                onChange={(event) => setTargetVersion(event.target.value)}
                placeholder='v1.1.0'
                disabled={
                  updating || updateActive || Boolean(expectedUpdateVersion)
                }
                aria-label={t('Target version or tag')}
              />
              <Button
                type='button'
                onClick={handleStartTargetVersion}
                disabled={
                  updating || updateActive || Boolean(expectedUpdateVersion)
                }
              >
                <DownloadIcon className='me-2 h-4 w-4' />
                {updating ? t('Starting update...') : t('Install selected tag')}
              </Button>
            </div>
          </div>

          {showUpdateTask && updateTask && (
            <div className='rounded-lg border p-4 md:w-1/2'>
              <div className='mb-4 flex items-start justify-between gap-3'>
                <div className='min-w-0'>
                  <div className='font-medium'>{t('System update')}</div>
                  <div className='text-muted-foreground truncate text-sm'>
                    {updateTask.state?.message ||
                      t('Waiting for the service to restart...')}
                  </div>
                </div>
                <div className='text-muted-foreground text-sm'>
                  {updateTask.state?.progress ?? 0}%
                </div>
              </div>
              <ol className='space-y-3'>
                {systemUpdateSteps.map((item, index) => {
                  const state = getSystemUpdateStepState(updateTask, index)
                  return (
                    <li key={item.step} className='flex items-start gap-3'>
                      <span
                        className={cn(
                          'mt-1 h-2.5 w-2.5 shrink-0 rounded-full',
                          state === 'idle' && 'bg-muted-foreground/40',
                          state === 'running' && 'bg-yellow-500',
                          state === 'done' && 'bg-green-500',
                          state === 'error' && 'bg-red-500'
                        )}
                        aria-hidden='true'
                      />
                      <span className='text-sm'>
                        {getSystemUpdateStepLabel(t, item.step)}
                      </span>
                    </li>
                  )
                })}
              </ol>
            </div>
          )}
        </div>
      </SettingsSection>

      <Dialog
        open={dialogOpen}
        onOpenChange={setDialogOpen}
        title={
          release?.tag_name
            ? t('New version available: {{version}}', {
                version: release.tag_name,
              })
            : t('Release details')
        }
        description={
          release?.published_at
            ? `${t('Published')} ${formatTimestampToDate(
                new Date(release.published_at).getTime(),
                'milliseconds'
              )}`
            : undefined
        }
        contentClassName='max-h-[80vh] overflow-y-auto'
        contentHeight='auto'
        bodyClassName='space-y-4'
        footer={
          <>
            <Button
              type='button'
              variant='secondary'
              onClick={() => setDialogOpen(false)}
            >
              {t('Close')}
            </Button>
            {release?.html_url && (
              <Button type='button' onClick={goToRelease}>
                <ExternalLinkIcon className='me-2 h-4 w-4' />
                {t('Open tag')}
              </Button>
            )}
            {release?.tag_name && (
              <Button
                type='button'
                onClick={handleStartUpdate}
                disabled={
                  !canUpdate ||
                  updating ||
                  updateActive ||
                  Boolean(expectedUpdateVersion)
                }
              >
                <DownloadIcon className='me-2 h-4 w-4' />
                {updating ? t('Starting update...') : t('Update now')}
              </Button>
            )}
          </>
        }
      >
        <div className='space-y-4'>
          {releases.length > 0 ? (
            [...releases]
              .reverse()
              .map((item) => (
                <div key={item.tag_name} className='space-y-2'>
                  <h3 className='text-base font-semibold'>{item.tag_name}</h3>
                  {item.body ? (
                    <Markdown>{item.body}</Markdown>
                  ) : (
                    <p className='text-muted-foreground text-sm'>
                      {t('No release notes provided.')}
                    </p>
                  )}
                </div>
              ))
          ) : (
            <p className='text-muted-foreground text-sm'>
              {t('No release notes provided.')}
            </p>
          )}
        </div>
      </Dialog>
    </>
  )
}
