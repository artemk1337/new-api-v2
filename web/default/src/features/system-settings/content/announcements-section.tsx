import { isAxiosError } from 'axios'
import { Plus, Save, Trash2 } from 'lucide-react'
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
import { useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { StaticDataTable } from '@/components/data-table/static/static-data-table'
import { StaticRowActions } from '@/components/data-table/static/static-row-actions'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from '@/components/ui/alert-dialog'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import dayjs from '@/lib/dayjs'

import { SettingsSwitchField } from '../components/settings-form-layout'
import { SettingsSection } from '../components/settings-section'
import { useUpdateOption } from '../hooks/use-update-option'
import { AnnouncementDelivery } from './announcement-delivery'
import { AnnouncementEditor } from './announcement-editor'
import {
  useRetryAnnouncement,
  useSaveAnnouncements,
  type Announcement,
  type AnnouncementSettings,
} from './announcements-api'

export function AnnouncementsSection(props: {
  enabled: boolean
  settings: AnnouncementSettings
  onRefresh: () => Promise<void>
}) {
  const { t } = useTranslation()
  const updateOption = useUpdateOption()
  const save = useSaveAnnouncements()
  const retry = useRetryAnnouncement()
  const [draft, setDraft] = useState<AnnouncementSettings | null>(null)
  const [selectedIds, setSelectedIds] = useState<string[]>([])
  const [editor, setEditor] = useState<{
    announcement: Announcement | null
    snapshot: AnnouncementSettings
  } | null>(null)
  const [deleteIds, setDeleteIds] = useState<string[]>([])
  const [unknownRetry, setUnknownRetry] = useState<Announcement | null>(null)
  const [conflict, setConflict] = useState(false)
  const settings = draft || props.settings
  const busy = save.isPending || retry.isPending
  const sortedAnnouncements = useMemo(
    () =>
      [...settings.announcements].sort(
        (a, b) => Date.parse(b.publishDate) - Date.parse(a.publishDate)
      ),
    [settings.announcements]
  )

  async function saveAll() {
    if (
      settings.channel &&
      !/^@[A-Za-z][A-Za-z0-9_]{4,31}$/.test(settings.channel)
    ) {
      toast.error(t('Enter a public Telegram channel username'))
      return
    }
    try {
      await save.mutateAsync(settings)
      setDraft(null)
      setConflict(false)
      toast.success(t('Announcements saved successfully'))
    } catch (error) {
      if (isAxiosError(error) && error.response?.status === 409) {
        setConflict(true)
      } else {
        toast.error(
          t(
            error instanceof Error
              ? error.message
              : 'Failed to save announcements'
          )
        )
      }
    }
  }

  async function retryPublication(item: Announcement, confirmUnknown = false) {
    if (item.telegramStatus === 'unknown' && !confirmUnknown) {
      setUnknownRetry(item)
      return
    }
    try {
      await retry.mutateAsync({ id: item.id, confirmUnknown })
      setUnknownRetry(null)
      toast.success(t('Telegram publication queued'))
    } catch (error) {
      toast.error(
        t(
          error instanceof Error
            ? error.message
            : 'Failed to retry Telegram publication'
        )
      )
    }
  }

  function submitAnnouncement(item: Announcement) {
    const snapshot = editor?.snapshot || settings
    const announcements = [...snapshot.announcements]
    const index = announcements.findIndex((existing) => existing.id === item.id)
    if (index < 0) announcements.push(item)
    else announcements[index] = item
    setDraft({ ...snapshot, announcements })
    setEditor(null)
    toast.success(t('Announcement updated. Click "Save Settings" to apply.'))
  }

  function confirmDelete() {
    setDraft({
      ...settings,
      announcements: settings.announcements.filter(
        (item) => !deleteIds.includes(item.id)
      ),
    })
    setSelectedIds((previous) =>
      previous.filter((id) => !deleteIds.includes(id))
    )
    setDeleteIds([])
  }

  return (
    <SettingsSection title={t('Announcements')}>
      <div className='space-y-5'>
        <div className='space-y-2'>
          <Label htmlFor='announcement-telegram-channel'>
            {t('Telegram channel')}
          </Label>
          <Input
            id='announcement-telegram-channel'
            value={settings.channel}
            placeholder='@channel'
            className='max-w-sm'
            disabled={busy}
            onChange={(event) =>
              setDraft({ ...settings, channel: event.target.value.trim() })
            }
          />
          <p className='text-muted-foreground text-sm'>
            {t(
              'Add the main bot as a channel administrator with permission to publish and edit posts.'
            )}
          </p>
          <p className='text-muted-foreground text-sm'>
            {t(
              'The existing bot token is used. Telegram sign-in can remain disabled.'
            )}
          </p>
        </div>
        {conflict && (
          <Alert variant='destructive'>
            <AlertTitle>{t('Announcements changed on the server')}</AlertTitle>
            <AlertDescription>
              <p>
                {t(
                  'Your changes are preserved. Reload the latest news before editing again.'
                )}
              </p>
              <Button
                variant='outline'
                size='sm'
                onClick={async () => {
                  try {
                    await props.onRefresh()
                    setDraft(null)
                    setConflict(false)
                    setSelectedIds([])
                  } catch {
                    toast.error(t('Failed to load announcements'))
                  }
                }}
              >
                {t('Discard changes and reload')}
              </Button>
            </AlertDescription>
          </Alert>
        )}
        <div className='flex flex-wrap items-center justify-between gap-3'>
          <div className='flex flex-wrap items-center gap-2'>
            <Button
              size='sm'
              disabled={busy}
              onClick={() =>
                setEditor({ announcement: null, snapshot: settings })
              }
            >
              <Plus className='mr-2 size-4' />
              {t('Add Announcement')}
            </Button>
            <Button
              size='sm'
              variant='destructive'
              disabled={!selectedIds.length || busy}
              onClick={() => setDeleteIds(selectedIds)}
            >
              <Trash2 className='mr-2 size-4' />
              {t('Delete (')}
              {selectedIds.length})
            </Button>
            <Button
              size='sm'
              variant='secondary'
              disabled={!draft || busy || !!editor || conflict}
              onClick={() => void saveAll()}
            >
              <Save className='mr-2 size-4' />
              {save.isPending ? t('Saving...') : t('Save Settings')}
            </Button>
          </div>
          <SettingsSwitchField
            checked={props.enabled}
            onCheckedChange={(checked) =>
              updateOption.mutate({
                key: 'console_setting.announcements_enabled',
                value: checked,
              })
            }
            label={t('Enabled')}
            className='py-0'
          />
        </div>
        {draft && (
          <p className='text-muted-foreground text-sm'>
            {t(
              'Save settings to publish changes. Existing Telegram posts are kept when news is deleted.'
            )}
          </p>
        )}
        <StaticDataTable
          data={sortedAnnouncements}
          getRowKey={(item) => item.id}
          emptyContent={t(
            'No announcements yet. Click "Add Announcement" to create one.'
          )}
          columns={[
            {
              id: 'select',
              className: 'w-12',
              header: (
                <Checkbox
                  aria-label={t('Select all')}
                  checked={
                    settings.announcements.length > 0 &&
                    selectedIds.length === settings.announcements.length
                  }
                  onCheckedChange={(checked) =>
                    setSelectedIds(
                      checked
                        ? settings.announcements.map((item) => item.id)
                        : []
                    )
                  }
                />
              ),
              cell: (item) => (
                <Checkbox
                  aria-label={t('Select announcement')}
                  checked={selectedIds.includes(item.id)}
                  onCheckedChange={(checked) =>
                    setSelectedIds((previous) =>
                      checked
                        ? [...previous, item.id]
                        : previous.filter((id) => id !== item.id)
                    )
                  }
                />
              ),
            },
            {
              id: 'content',
              header: t('Content'),
              cellClassName: 'max-w-xs truncate',
              cell: (item) => item.content,
            },
            {
              id: 'publish-date',
              header: t('Publish Date'),
              cell: (item) => (
                <span className='text-sm whitespace-nowrap'>
                  {dayjs(item.publishDate).format('YYYY-MM-DD HH:mm')}
                </span>
              ),
            },
            {
              id: 'telegram',
              header: t('Telegram publication'),
              cell: (item) => (
                <AnnouncementDelivery
                  announcement={item}
                  disabled={!!draft || busy}
                  onRetry={(announcement) =>
                    void retryPublication(announcement)
                  }
                />
              ),
            },
            {
              id: 'actions',
              header: t('Actions'),
              cell: (item) => (
                <StaticRowActions
                  editLabel={t('Edit')}
                  deleteLabel={t('Delete')}
                  menuLabel={t('Open menu')}
                  editDisabled={busy}
                  deleteDisabled={busy}
                  onEdit={() =>
                    setEditor({ announcement: item, snapshot: settings })
                  }
                  onDelete={() => setDeleteIds([item.id])}
                />
              ),
            },
          ]}
        />
      </div>
      {editor && (
        <AnnouncementEditor
          announcement={editor.announcement}
          onClose={() => setEditor(null)}
          onSubmit={submitAnnouncement}
        />
      )}
      <AlertDialog
        open={deleteIds.length > 0}
        onOpenChange={(open) => {
          if (!open) setDeleteIds([])
        }}
      >
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>{t('Are you sure?')}</AlertDialogTitle>
            <AlertDialogDescription>
              {t(
                'Selected news will be removed after saving. Published Telegram posts will remain in the channel.'
              )}
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>{t('Cancel')}</AlertDialogCancel>
            <AlertDialogAction variant='destructive' onClick={confirmDelete}>
              {t('Delete')}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
      <AlertDialog
        open={!!unknownRetry}
        onOpenChange={(open) => {
          if (!open && !retry.isPending) setUnknownRetry(null)
        }}
      >
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>
              {t('Check the Telegram channel before retrying')}
            </AlertDialogTitle>
            <AlertDialogDescription>
              {t(
                'The previous post may already exist. Retrying can create a duplicate. Link the existing post using Edit, or confirm another attempt.'
              )}
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel disabled={retry.isPending}>
              {t('Cancel')}
            </AlertDialogCancel>
            <AlertDialogAction
              disabled={retry.isPending}
              onClick={(event) => {
                event.preventDefault()
                if (unknownRetry) void retryPublication(unknownRetry, true)
              }}
            >
              {t('Confirm Telegram retry')}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </SettingsSection>
  )
}
