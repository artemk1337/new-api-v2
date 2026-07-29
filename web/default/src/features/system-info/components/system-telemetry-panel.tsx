import { useQuery } from '@tanstack/react-query'
import { Activity, Play, Square } from 'lucide-react'
import { useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Line, LineChart, CartesianGrid, XAxis, YAxis } from 'recharts'

import { ConfirmDialog } from '@/components/confirm-dialog'
import { ErrorState } from '@/components/error-state'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  ChartContainer,
  ChartTooltip,
  ChartTooltipContent,
} from '@/components/ui/chart'
import { Skeleton } from '@/components/ui/skeleton'
import { formatTimestampToDate } from '@/lib/format'

import {
  getSystemTelemetry,
  getSystemTelemetryAgent,
  listSystemInstances,
  startSystemTelemetryAgent,
  stopSystemTelemetryAgent,
} from '../api'
import type { SystemTelemetryProcess } from '../types'

const REFRESH_INTERVAL_MS = 30_000
const RANGES: Array<1 | 6 | 24> = [1, 6, 24]

function formatBytes(bytes: number) {
  return `${(bytes / 1024 / 1024).toFixed(1)} MB`
}

export function SystemTelemetryPanel() {
  const { t } = useTranslation()
  const [hours, setHours] = useState<1 | 6 | 24>(24)
  const [selectedNode, setSelectedNode] = useState('')
  const [stopping, setStopping] = useState(false)
  const [updating, setUpdating] = useState(false)
  const instancesQuery = useQuery({
    queryKey: ['system-info', 'instances'],
    queryFn: async () => {
      const res = await listSystemInstances()
      if (!res.success || !res.data) throw new Error(res.message)
      return res.data
    },
    staleTime: REFRESH_INTERVAL_MS,
  })
  const nodeName = selectedNode || instancesQuery.data?.[0]?.node_name || ''
  const telemetryQuery = useQuery({
    queryKey: ['system-info', 'telemetry', nodeName, hours],
    queryFn: async () => {
      const res = await getSystemTelemetry(nodeName, hours)
      if (!res.success || !res.data) throw new Error(res.message)
      return res.data
    },
    enabled: nodeName !== '',
    staleTime: REFRESH_INTERVAL_MS,
    refetchInterval: REFRESH_INTERVAL_MS,
  })
  const agentQuery = useQuery({
    queryKey: ['system-info', 'telemetry-agent'],
    queryFn: getSystemTelemetryAgent,
    staleTime: REFRESH_INTERVAL_MS,
    refetchInterval: REFRESH_INTERVAL_MS,
  })
  const samples = telemetryQuery.data ?? []
  const latest = samples.at(-1)
  const leaders = useMemo(() => {
    const byName = new Map<string, SystemTelemetryProcess>()
    for (const sample of samples) {
      for (const process of sample.top_processes ?? []) {
        const current = byName.get(process.name)
        if (!current || process.cpu_usage > current.cpu_usage) {
          byName.set(process.name, process)
        }
      }
    }
    return [...byName.values()]
      .sort((a, b) => b.cpu_usage - a.cpu_usage)
      .slice(0, 3)
  }, [samples])
  const chartData = samples.map((sample) => ({
    time: new Date(sample.collected_at * 1000).toLocaleTimeString([], {
      hour: '2-digit',
      minute: '2-digit',
    }),
    cpu: sample.cpu_usage,
    memory: sample.memory_usage,
    swap: sample.swap_usage,
    iowait: sample.io_wait,
    load: sample.load_average_1,
  }))

  async function updateAgent(action: 'start' | 'stop') {
    setUpdating(true)
    try {
      if (action === 'start') await startSystemTelemetryAgent()
      else await stopSystemTelemetryAgent()
      await Promise.all([agentQuery.refetch(), telemetryQuery.refetch()])
    } finally {
      setUpdating(false)
      setStopping(false)
    }
  }

  return (
    <section className='bg-card overflow-hidden rounded-lg border shadow-xs'>
      <div className='flex flex-col gap-3 border-b px-4 py-3 sm:flex-row sm:items-center sm:justify-between sm:px-5'>
        <div className='flex items-center gap-2'>
          <span className='bg-muted text-muted-foreground inline-flex size-7 items-center justify-center rounded-md'>
            <Activity className='size-4' aria-hidden='true' />
          </span>
          <div>
            <h3 className='text-sm font-semibold'>{t('System telemetry')}</h3>
            <p className='text-muted-foreground text-xs'>
              {t('Host resource history and CPU leaders.')}
            </p>
          </div>
        </div>
        <div className='flex items-center gap-2'>
          <Badge variant='outline'>
            {agentQuery.data?.running ? t('running') : t('stopped')}
          </Badge>
          {agentQuery.data?.running ? (
            <Button
              size='sm'
              variant='outline'
              onClick={() => setStopping(true)}
              disabled={updating}
            >
              <Square data-icon='inline-start' />
              {t('Stop')}
            </Button>
          ) : (
            <Button
              size='sm'
              onClick={() => void updateAgent('start')}
              disabled={updating}
            >
              <Play data-icon='inline-start' />
              {t('Start')}
            </Button>
          )}
        </div>
      </div>
      <div className='space-y-4 p-4 sm:p-5'>
        <div className='flex flex-wrap gap-2'>
          <select
            className='border-input h-8 rounded-lg border bg-transparent px-2 text-sm'
            value={nodeName}
            onChange={(event) => setSelectedNode(event.target.value)}
          >
            {instancesQuery.data?.map((instance) => (
              <option key={instance.node_name} value={instance.node_name}>
                {instance.node_name}
              </option>
            ))}
          </select>
          {RANGES.map((range) => (
            <Button
              key={range}
              size='sm'
              variant={hours === range ? 'default' : 'outline'}
              onClick={() => setHours(range)}
            >
              {range}h
            </Button>
          ))}
        </div>
        {telemetryQuery.isLoading ? (
          <Skeleton className='h-72 w-full' />
        ) : telemetryQuery.isError ? (
          <ErrorState title={t('We could not load telemetry.')} />
        ) : samples.length === 0 ? (
          <p className='text-muted-foreground py-8 text-center text-sm'>
            {t('No telemetry data yet.')}
          </p>
        ) : (
          <>
            <ChartContainer
              className='h-64 w-full'
              config={{
                cpu: { label: t('CPU'), color: 'var(--chart-1)' },
                memory: { label: t('Memory'), color: 'var(--chart-2)' },
                swap: { label: t('Swap'), color: 'var(--chart-3)' },
              }}
            >
              <LineChart data={chartData}>
                <CartesianGrid vertical={false} />
                <XAxis dataKey='time' minTickGap={50} />
                <YAxis unit='%' />
                <ChartTooltip content={<ChartTooltipContent />} />
                <Line
                  type='monotone'
                  dataKey='cpu'
                  stroke='var(--color-cpu)'
                  dot={false}
                />
                <Line
                  type='monotone'
                  dataKey='memory'
                  stroke='var(--color-memory)'
                  dot={false}
                />
                <Line
                  type='monotone'
                  dataKey='swap'
                  stroke='var(--color-swap)'
                  dot={false}
                />
              </LineChart>
            </ChartContainer>
            <ChartContainer
              className='h-48 w-full'
              config={{
                iowait: { label: t('I/O wait'), color: 'var(--chart-4)' },
                load: { label: t('Load average'), color: 'var(--chart-5)' },
              }}
            >
              <LineChart data={chartData}>
                <CartesianGrid vertical={false} />
                <XAxis dataKey='time' minTickGap={50} />
                <YAxis yAxisId='percent' unit='%' />
                <YAxis yAxisId='load' orientation='right' allowDecimals />
                <ChartTooltip content={<ChartTooltipContent />} />
                <Line
                  yAxisId='percent'
                  type='monotone'
                  dataKey='iowait'
                  stroke='var(--color-iowait)'
                  dot={false}
                />
                <Line
                  yAxisId='load'
                  type='monotone'
                  dataKey='load'
                  stroke='var(--color-load)'
                  dot={false}
                />
              </LineChart>
            </ChartContainer>
            <div className='grid gap-3 sm:grid-cols-2'>
              <div className='rounded-md border p-3'>
                <div className='mb-2 text-sm font-medium'>
                  {t('Current top processes')}
                </div>
                {(latest?.top_processes ?? []).map((process) => (
                  <div
                    key={process.pid}
                    className='flex justify-between font-mono text-xs'
                  >
                    <span>
                      {process.name} #{process.pid}
                    </span>
                    <span>
                      {process.cpu_usage.toFixed(1)}% ·{' '}
                      {formatBytes(process.rss_bytes)}
                    </span>
                  </div>
                ))}
              </div>
              <div className='rounded-md border p-3'>
                <div className='mb-2 text-sm font-medium'>
                  {t('CPU leaders')}
                </div>
                {leaders.map((process) => (
                  <div
                    key={`${process.name}-${process.pid}`}
                    className='flex justify-between font-mono text-xs'
                  >
                    <span>{process.name}</span>
                    <span>{process.cpu_usage.toFixed(1)}%</span>
                  </div>
                ))}
              </div>
            </div>
            <p className='text-muted-foreground text-xs'>
              {t('Last sample:')}{' '}
              {latest ? formatTimestampToDate(latest.collected_at) : '-'}
            </p>
          </>
        )}
      </div>
      <ConfirmDialog
        open={stopping}
        onOpenChange={setStopping}
        title={t('Stop telemetry agent?')}
        desc={t(
          'The history remains available, but new host metrics will not be collected.'
        )}
        confirmText={t('Stop')}
        destructive
        isLoading={updating}
        handleConfirm={() => void updateAgent('stop')}
      />
    </section>
  )
}
