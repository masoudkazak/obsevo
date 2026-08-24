<script lang="ts">
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
						label: 'Total Cost ($)',
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
							label: 'Avg Latency',
							data: latencyData.map(d => d.avg_latency_seconds),
							borderColor: '#8b5cf6',
							backgroundColor: 'rgba(139,92,246,0.1)',
							fill: true,
							tension: 0.3
						},
						{
							label: 'Max Latency',
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
							label: 'Input Tokens',
							data: tokenData.map(d => d.total_input_tokens),
							backgroundColor: '#06b6d4'
						},
						{
							label: 'Output Tokens',
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
							label: 'Total Traces',
							data: traceData.map(d => d.trace_count),
							backgroundColor: '#10b981'
						},
						{
							label: 'Error Traces',
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
	<title>Analytics — Langfuse Light</title>
</svelte:head>

<div class="max-w-7xl mx-auto">
	<div class="flex items-center justify-between mb-6">
		<h1 class="text-2xl font-bold text-gray-900">Analytics</h1>
		<select
			bind:value={days}
			onchange={() => loadData()}
			class="px-3 py-2 border border-gray-300 rounded-md text-sm focus:outline-none focus:ring-2 focus:ring-blue-500"
		>
			<option value={7}>Last 7 days</option>
			<option value={14}>Last 14 days</option>
			<option value={30}>Last 30 days</option>
			<option value={90}>Last 90 days</option>
		</select>
	</div>

	{#if loading}
		<div class="text-center py-12 text-gray-500">Loading analytics...</div>
	{:else}
		<!-- Summary Cards -->
		<div class="grid grid-cols-2 md:grid-cols-4 gap-4 mb-8">
			<div class="bg-white rounded-lg border border-gray-200 p-4">
				<div class="text-sm text-gray-500">Total Traces</div>
				<div class="text-2xl font-bold text-gray-900">{summary?.latency.total_traces ?? 0}</div>
			</div>
			<div class="bg-white rounded-lg border border-gray-200 p-4">
				<div class="text-sm text-gray-500">Total Cost</div>
				<div class="text-2xl font-bold text-gray-900">{formatCost(summary?.cost.total_cost ?? 0)}</div>
			</div>
			<div class="bg-white rounded-lg border border-gray-200 p-4">
				<div class="text-sm text-gray-500">Avg Latency</div>
				<div class="text-2xl font-bold text-gray-900">{formatDuration(summary?.latency.avg_latency_seconds ?? 0)}</div>
			</div>
			<div class="bg-white rounded-lg border border-gray-200 p-4">
				<div class="text-sm text-gray-500">Error Rate</div>
				<div class="text-2xl font-bold text-gray-900">{((summary?.error_rate.error_rate ?? 0) * 100).toFixed(1)}%</div>
			</div>
		</div>

		<div class="grid grid-cols-1 md:grid-cols-2 gap-6 mb-6">
			<!-- Cost Chart -->
			<div class="bg-white rounded-lg border border-gray-200 p-4">
				<h3 class="text-sm font-medium text-gray-700 mb-3">Cost Over Time</h3>
				<div class="h-64">
					<canvas id="costChart"></canvas>
				</div>
			</div>

			<!-- Latency Chart -->
			<div class="bg-white rounded-lg border border-gray-200 p-4">
				<h3 class="text-sm font-medium text-gray-700 mb-3">Latency Over Time</h3>
				<div class="h-64">
					<canvas id="latencyChart"></canvas>
				</div>
			</div>

			<!-- Token Usage Chart -->
			<div class="bg-white rounded-lg border border-gray-200 p-4">
				<h3 class="text-sm font-medium text-gray-700 mb-3">Token Usage</h3>
				<div class="h-64">
					<canvas id="tokenChart"></canvas>
				</div>
			</div>

			<!-- Trace Count Chart -->
			<div class="bg-white rounded-lg border border-gray-200 p-4">
				<h3 class="text-sm font-medium text-gray-700 mb-3">Traces & Errors</h3>
				<div class="h-64">
					<canvas id="traceChart"></canvas>
				</div>
			</div>
		</div>

		<!-- Token Summary -->
		<div class="bg-white rounded-lg border border-gray-200 p-4">
			<h3 class="text-sm font-medium text-gray-700 mb-3">Token Usage Summary</h3>
			<div class="grid grid-cols-3 gap-4">
				<div>
					<div class="text-xs text-gray-500">Total Tokens</div>
					<div class="text-lg font-semibold">{formatTokens(summary?.token_usage.total_tokens ?? 0)}</div>
				</div>
				<div>
					<div class="text-xs text-gray-500">Input Tokens</div>
					<div class="text-lg font-semibold">{formatTokens(summary?.token_usage.total_input_tokens ?? 0)}</div>
				</div>
				<div>
					<div class="text-xs text-gray-500">Output Tokens</div>
					<div class="text-lg font-semibold">{formatTokens(summary?.token_usage.total_output_tokens ?? 0)}</div>
				</div>
			</div>
		</div>
	{/if}
</div>
