<script lang="ts">
	import { t } from '$lib/i18n';
	import { onMount } from 'svelte';
	import { api, type AnalyticsSummary, type CostOverTimePoint, type LatencyOverTimePoint, type TokenUsageOverTimePoint, type TraceCountOverTimePoint } from '$lib/api';
	import { currentProject, notifications } from '$lib/stores';
	import Chart from 'chart.js/auto';

	let summary = $state<AnalyticsSummary | null>(null);
	let costData = $state<CostOverTimePoint[]>([]);
	let latencyData = $state<LatencyOverTimePoint[]>([]);
	let tokenData = $state<TokenUsageOverTimePoint[]>([]);
	let traceData = $state<TraceCountOverTimePoint[]>([]);
	let loading = $state(true);
	let days = $state(30);

	let costChart: Chart | null = null;
	let latencyChart: Chart | null = null;
	let tokenChart: Chart | null = null;
	let traceChart: Chart | null = null;

	function formatCost(v: number): string {
		return `$${v.toFixed(4)}`;
	}

	function formatDuration(s: number): string {
		if (s < 1) return `${(s * 1000).toFixed(0)}ms`;
		return `${s.toFixed(2)}s`;
	}

	function formatTokens(n: number): string {
		if (n >= 1_000_000) return `${(n / 1_000_000).toFixed(1)}M`;
		if (n >= 1_000) return `${(n / 1_000).toFixed(1)}K`;
		return String(n);
	}

	async function loadData() {
		if (!$currentProject) return;
		loading = true;
		try {
			const [s, c, l, t, tr] = await Promise.all([
				api.analytics.summary($currentProject.id),
				api.analytics.costOverTime($currentProject.id, days),
				api.analytics.latencyOverTime($currentProject.id, days),
				api.analytics.tokensOverTime($currentProject.id, days),
				api.analytics.tracesOverTime($currentProject.id, days)
			]);
			summary = s;
			costData = c;
			latencyData = l;
			tokenData = t;
			traceData = tr;
			renderCharts();
		} catch (e: any) {
			notifications.error(e.message || 'Failed to load analytics');
		} finally {
			loading = false;
		}
	}

	function renderCharts() {
		const labels = costData.map(d => d.time_bucket);

		if (costChart) costChart.destroy();
		const costCtx = document.getElementById('costChart') as HTMLCanvasElement;
		if (costCtx) {
			costChart = new Chart(costCtx, {
				type: 'line',
				data: {
					labels,
					datasets: [{
						label: $t('analytics.datasetLabels.totalCost'),
						data: costData.map(d => d.total_cost),
						borderColor: '#3b82f6',
						backgroundColor: 'rgba(59,130,246,0.1)',
						fill: true,
						tension: 0.3
					}]
				},
				options: {
					responsive: true,
					maintainAspectRatio: false,
					plugins: { legend: { display: false } },
					scales: {
						y: { beginAtZero: true, ticks: { callback: (v) => `$${v}` } }
					}
				}
			});
		}

		if (latencyChart) latencyChart.destroy();
		const latCtx = document.getElementById('latencyChart') as HTMLCanvasElement;
		if (latCtx) {
			latencyChart = new Chart(latCtx, {
				type: 'line',
				data: {
					labels: latencyData.map(d => d.time_bucket),
					datasets: [
						{
							label: $t('analytics.datasetLabels.avgLatency'),
							data: latencyData.map(d => d.avg_latency_seconds),
							borderColor: '#8b5cf6',
							backgroundColor: 'rgba(139,92,246,0.1)',
							fill: true,
							tension: 0.3
						},
						{
							label: $t('analytics.datasetLabels.maxLatency'),
							data: latencyData.map(d => d.max_latency_seconds),
							borderColor: '#ef4444',
							borderDash: [5, 5],
							fill: false,
							tension: 0.3
						}
					]
				},
				options: {
					responsive: true,
					maintainAspectRatio: false,
					scales: {
						y: { beginAtZero: true, ticks: { callback: (v) => formatDuration(Number(v)) } }
					}
				}
			});
		}

		if (tokenChart) tokenChart.destroy();
		const tokCtx = document.getElementById('tokenChart') as HTMLCanvasElement;
		if (tokCtx) {
			tokenChart = new Chart(tokCtx, {
				type: 'bar',
				data: {
					labels: tokenData.map(d => d.time_bucket),
					datasets: [
						{
							label: $t('analytics.datasetLabels.inputTokens'),
							data: tokenData.map(d => d.total_input_tokens),
							backgroundColor: '#06b6d4'
						},
						{
							label: $t('analytics.datasetLabels.outputTokens'),
							data: tokenData.map(d => d.total_output_tokens),
							backgroundColor: '#f59e0b'
						}
					]
				},
				options: {
					responsive: true,
					maintainAspectRatio: false,
					scales: {
						x: { stacked: true },
						y: { stacked: true, beginAtZero: true, ticks: { callback: (v) => formatTokens(Number(v)) } }
					}
				}
			});
		}

		if (traceChart) traceChart.destroy();
		const trCtx = document.getElementById('traceChart') as HTMLCanvasElement;
		if (trCtx) {
			traceChart = new Chart(trCtx, {
				type: 'bar',
				data: {
					labels: traceData.map(d => d.time_bucket),
					datasets: [
						{
							label: $t('analytics.datasetLabels.totalTraces'),
							data: traceData.map(d => d.trace_count),
							backgroundColor: '#10b981'
						},
						{
							label: $t('analytics.datasetLabels.errorTraces'),
							data: traceData.map(d => d.error_count),
							backgroundColor: '#ef4444'
						}
					]
				},
				options: {
					responsive: true,
					maintainAspectRatio: false,
					scales: { y: { beginAtZero: true } }
				}
			});
		}
	}

	$effect(() => {
		if ($currentProject) {
			loadData();
		} else {
			loading = false;
		}
	});

	onMount(() => {
		return () => {
			costChart?.destroy();
			latencyChart?.destroy();
			tokenChart?.destroy();
			traceChart?.destroy();
		};
	});
</script>

<svelte:head>
	<title>{$t('analytics.title')}</title>
</svelte:head>

<div class="max-w-7xl mx-auto">
	<div class="flex items-center justify-between mb-6">
		<h1 class="text-2xl font-bold text-gray-900">{$t('analytics.heading')}</h1>
		<select
			bind:value={days}
			onchange={() => loadData()}
			class="px-3 py-2 border border-gray-300 rounded-md text-sm focus:outline-none focus:ring-2 focus:ring-blue-500"
		>
			<option value={7}>{$t('analytics.days7')}</option>
			<option value={14}>{$t('analytics.days14')}</option>
			<option value={30}>{$t('analytics.days30')}</option>
			<option value={90}>{$t('analytics.days90')}</option>
		</select>
	</div>

	{#if loading}
		<div class="text-center py-12 text-gray-500">{$t('analytics.loadingAnalytics')}</div>
	{:else if !$currentProject}
		<div class="text-center py-12 bg-white rounded-lg border border-gray-200">
			<h3 class="text-sm font-medium text-gray-900">{$t('common.noProject')}</h3>
			<p class="mt-1 text-sm text-gray-500">{$t('common.noProjectDesc')}</p>
		</div>
	{:else}
		<!-- Summary Cards -->
		<div class="grid grid-cols-2 md:grid-cols-4 gap-4 mb-8">
			<div class="bg-white rounded-lg border border-gray-200 p-4">
				<div class="text-sm text-gray-500">{$t('analytics.summary.totalTraces')}</div>
				<div class="text-2xl font-bold text-gray-900">{summary?.latency.total_traces ?? 0}</div>
			</div>
			<div class="bg-white rounded-lg border border-gray-200 p-4">
				<div class="text-sm text-gray-500">{$t('analytics.summary.totalCost')}</div>
				<div class="text-2xl font-bold text-gray-900">{formatCost(summary?.cost.total_cost ?? 0)}</div>
			</div>
			<div class="bg-white rounded-lg border border-gray-200 p-4">
				<div class="text-sm text-gray-500">{$t('analytics.summary.avgLatency')}</div>
				<div class="text-2xl font-bold text-gray-900">{formatDuration(summary?.latency.avg_latency_seconds ?? 0)}</div>
			</div>
			<div class="bg-white rounded-lg border border-gray-200 p-4">
				<div class="text-sm text-gray-500">{$t('analytics.summary.errorRate')}</div>
				<div class="text-2xl font-bold text-gray-900">{((summary?.error_rate.error_rate ?? 0) * 100).toFixed(1)}%</div>
			</div>
		</div>

		<div class="grid grid-cols-1 md:grid-cols-2 gap-6 mb-6">
			<!-- Cost Chart -->
			<div class="bg-white rounded-lg border border-gray-200 p-4">
				<h3 class="text-sm font-medium text-gray-700 mb-3">{$t('analytics.charts.costOverTime')}</h3>
				<div class="h-64">
					<canvas id="costChart"></canvas>
				</div>
			</div>

			<!-- Latency Chart -->
			<div class="bg-white rounded-lg border border-gray-200 p-4">
				<h3 class="text-sm font-medium text-gray-700 mb-3">{$t('analytics.charts.latencyOverTime')}</h3>
				<div class="h-64">
					<canvas id="latencyChart"></canvas>
				</div>
			</div>

			<!-- Token Usage Chart -->
			<div class="bg-white rounded-lg border border-gray-200 p-4">
				<h3 class="text-sm font-medium text-gray-700 mb-3">{$t('analytics.charts.tokenUsage')}</h3>
				<div class="h-64">
					<canvas id="tokenChart"></canvas>
				</div>
			</div>

			<!-- Trace Count Chart -->
			<div class="bg-white rounded-lg border border-gray-200 p-4">
				<h3 class="text-sm font-medium text-gray-700 mb-3">{$t('analytics.charts.tracesAndErrors')}</h3>
				<div class="h-64">
					<canvas id="traceChart"></canvas>
				</div>
			</div>
		</div>

		<!-- Token Summary -->
		<div class="bg-white rounded-lg border border-gray-200 p-4">
			<h3 class="text-sm font-medium text-gray-700 mb-3">{$t('analytics.tokenSummary')}</h3>
			<div class="grid grid-cols-3 gap-4">
				<div>
					<div class="text-xs text-gray-500">{$t('analytics.totalTokens')}</div>
					<div class="text-lg font-semibold">{formatTokens(summary?.token_usage.total_tokens ?? 0)}</div>
				</div>
				<div>
					<div class="text-xs text-gray-500">{$t('analytics.inputTokens')}</div>
					<div class="text-lg font-semibold">{formatTokens(summary?.token_usage.total_input_tokens ?? 0)}</div>
				</div>
				<div>
					<div class="text-xs text-gray-500">{$t('analytics.outputTokens')}</div>
					<div class="text-lg font-semibold">{formatTokens(summary?.token_usage.total_output_tokens ?? 0)}</div>
				</div>
			</div>
		</div>
	{/if}
</div>
