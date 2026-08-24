<script lang="ts">
	import { page } from '$app/state';
	import { api, type TraceWithObservations, type Observation } from '$lib/api';
	import { currentProject, notifications } from '$lib/stores';

	let data = $state<TraceWithObservations | null>(null);
	let loading = $state(true);
	let error = $state('');
	let expandedObs = $state<Set<string>>(new Set());

	$effect(() => {
		const id = page.params.id;
		if (id) loadTrace(id);
	});

	async function loadTrace(id: string) {
		loading = true;
		error = '';
		try {
			data = await api.traces.get(id);
		} catch (e: any) {
			error = e.message || 'Failed to load trace';
		} finally {
			loading = false;
		}
	}

	function toggleObs(id: string) {
		if (expandedObs.has(id)) {
			expandedObs.delete(id);
		} else {
			expandedObs.add(id);
		}
		expandedObs = expandedObs;
	}

	function formatTime(ts: string | null): string {
		if (!ts) return '—';
		return new Date(ts).toLocaleString();
	}

	function formatCost(cost: number | null): string {
		if (cost == null) return '—';
		return `$${cost.toFixed(6)}`;
	}

	function formatDuration(start: string | null, end: string | null): string {
		if (!start || !end) return '—';
		const ms = new Date(end).getTime() - new Date(start).getTime();
		if (ms < 1000) return `${ms}ms`;
		return `${(ms / 1000).toFixed(3)}s`;
	}

	function typeColor(type: string): string {
		switch (type) {
			case 'GENERATION': return 'bg-purple-100 text-purple-800';
			case 'SPAN': return 'bg-blue-100 text-blue-800';
			case 'EVENT': return 'bg-yellow-100 text-yellow-800';
			default: return 'bg-gray-100 text-gray-800';
		}
	}

	function buildTree(observations: Observation[]): Observation[] {
		const root: Observation[] = [];
		const map = new Map<string, Observation[]>();
		for (const obs of observations) {
			const pid = obs.parent_observation_id || '__root__';
			if (!map.has(pid)) map.set(pid, []);
			map.get(pid)!.push(obs);
		}
		function collect(pid: string): Observation[] {
			const children = map.get(pid) || [];
			const result: Observation[] = [];
			for (const child of children) {
				result.push(child);
				result.push(...collect(child.id));
			}
			return result;
		}
		return collect('__root__');
	}

	function treeDepth(obs: Observation, observations: Observation[]): number {
		let depth = 0;
		let current = obs;
		while (current.parent_observation_id) {
			depth++;
			current = observations.find(o => o.id === current.parent_observation_id) || current;
			if (depth > 20) break;
		}
		return depth;
	}

	let sortedObs = $derived(data ? buildTree(data.observations) : []);
</script>

<svelte:head>
	<title>{data?.trace?.name || 'Trace'} — Langfuse Light</title>
</svelte:head>

<div class="max-w-5xl mx-auto">
	<a href="/traces" class="text-sm text-blue-600 hover:underline mb-4 inline-block">&larr; Back to traces</a>

	{#if loading}
		<div class="text-center py-12 text-gray-500">Loading trace...</div>
	{:else if error}
		<div class="text-center py-12 text-red-500">{error}</div>
	{:else if data}
		<!-- Trace header -->
		<div class="bg-white rounded-lg border border-gray-200 p-6 mb-6">
			<h1 class="text-2xl font-bold text-gray-900 mb-4">{data.trace.name || data.trace.id}</h1>
			<div class="grid grid-cols-2 md:grid-cols-4 gap-4 text-sm">
				<div>
					<span class="text-gray-500">Trace ID</span>
					<p class="font-mono text-xs mt-1 truncate" title={data.trace.id}>{data.trace.id}</p>
				</div>
				<div>
					<span class="text-gray-500">Project</span>
					<p class="mt-1">{data.trace.project_name}</p>
				</div>
				<div>
					<span class="text-gray-500">Start</span>
					<p class="mt-1">{formatTime(data.trace.start_time)}</p>
				</div>
				<div>
					<span class="text-gray-500">Duration</span>
					<p class="mt-1">{formatDuration(data.trace.start_time, data.trace.end_time)}</p>
				</div>
				<div>
					<span class="text-gray-500">Cost</span>
					<p class="mt-1 font-medium">{formatCost(data.trace.total_cost)}</p>
				</div>
				<div>
					<span class="text-gray-500">User ID</span>
					<p class="mt-1">{data.trace.user_id || '—'}</p>
				</div>
				<div>
					<span class="text-gray-500">Session ID</span>
					<p class="mt-1">{data.trace.session_id || '—'}</p>
				</div>
				<div>
					<span class="text-gray-500">Tags</span>
					<p class="mt-1">
						{#each (data.trace.tags || []) as tag}
							<span class="inline-block px-2 py-0.5 bg-gray-100 text-gray-600 rounded text-xs mr-1">{tag}</span>
						{:else}
							—
						{/each}
					</p>
				</div>
			</div>
		</div>

		<!-- Observations -->
		<div class="bg-white rounded-lg border border-gray-200 p-6">
			<h2 class="text-lg font-semibold mb-4">Observations ({data.observations.length})</h2>
			{#if data.observations.length === 0}
				<p class="text-gray-500 text-sm">No observations recorded for this trace.</p>
			{:else}
				<div class="space-y-2">
					{#each sortedObs as obs}
						{@const depth = treeDepth(obs, data.observations)}
						<div class="border border-gray-100 rounded" style="margin-left: {depth * 24}px">
							<button
								class="w-full flex items-center gap-3 px-4 py-3 text-left hover:bg-gray-50"
								onclick={() => toggleObs(obs.id)}
							>
								<span class="text-gray-400 text-xs">{depth > 0 ? '└' : ''}</span>
								<span class="px-2 py-0.5 rounded text-xs font-medium {typeColor(obs.type)}">{obs.type}</span>
								<span class="text-sm font-medium text-gray-900">{obs.name || obs.id.slice(0, 8)}</span>
								{#if obs.model}
									<span class="text-xs text-gray-500">{obs.model}</span>
								{/if}
								<span class="text-xs text-gray-400 ml-auto">{formatDuration(obs.start_time, obs.end_time)}</span>
								<span class="text-xs text-gray-400">{formatCost(obs.cost)}</span>
								<svg class="w-4 h-4 text-gray-400 transition-transform {expandedObs.has(obs.id) ? 'rotate-90' : ''}" fill="none" stroke="currentColor" viewBox="0 0 24 24">
									<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 5l7 7-7 7" />
								</svg>
							</button>
							{#if expandedObs.has(obs.id)}
								<div class="px-4 pb-4 border-t border-gray-100">
									<div class="grid grid-cols-2 gap-4 mt-3 text-sm">
										<div>
											<span class="text-gray-500 text-xs">Status</span>
											<p class="mt-1">{obs.status}</p>
										</div>
										<div>
											<span class="text-gray-500 text-xs">Start</span>
											<p class="mt-1">{formatTime(obs.start_time)}</p>
										</div>
										<div>
											<span class="text-gray-500 text-xs">End</span>
											<p class="mt-1">{formatTime(obs.end_time)}</p>
										</div>
										<div>
											<span class="text-gray-500 text-xs">Cost</span>
											<p class="mt-1">{formatCost(obs.cost)}</p>
										</div>
									</div>
									{#if obs.input}
										<div class="mt-3">
											<span class="text-gray-500 text-xs">Input</span>
											<pre class="mt-1 p-3 bg-gray-50 rounded text-xs overflow-x-auto max-h-48">{JSON.stringify(obs.input, null, 2)}</pre>
										</div>
									{/if}
									{#if obs.output}
										<div class="mt-3">
											<span class="text-gray-500 text-xs">Output</span>
											<pre class="mt-1 p-3 bg-gray-50 rounded text-xs overflow-x-auto max-h-48">{JSON.stringify(obs.output, null, 2)}</pre>
										</div>
									{/if}
									{#if obs.token_usage}
										<div class="mt-3">
											<span class="text-gray-500 text-xs">Token Usage</span>
											<pre class="mt-1 p-3 bg-gray-50 rounded text-xs">{JSON.stringify(obs.token_usage, null, 2)}</pre>
										</div>
									{/if}
									{#if obs.metadata}
										<div class="mt-3">
											<span class="text-gray-500 text-xs">Metadata</span>
											<pre class="mt-1 p-3 bg-gray-50 rounded text-xs">{JSON.stringify(obs.metadata, null, 2)}</pre>
										</div>
									{/if}
								</div>
							{/if}
						</div>
					{/each}
				</div>
			{/if}
		</div>

		<!-- Trace input/output -->
		{#if data.trace.input || data.trace.output}
			<div class="bg-white rounded-lg border border-gray-200 p-6 mt-6">
				<h2 class="text-lg font-semibold mb-4">Trace I/O</h2>
				{#if data.trace.input}
					<div class="mb-4">
						<span class="text-gray-500 text-xs">Input</span>
						<pre class="mt-1 p-3 bg-gray-50 rounded text-xs overflow-x-auto max-h-48">{JSON.stringify(data.trace.input, null, 2)}</pre>
					</div>
				{/if}
				{#if data.trace.output}
					<div>
						<span class="text-gray-500 text-xs">Output</span>
						<pre class="mt-1 p-3 bg-gray-50 rounded text-xs overflow-x-auto max-h-48">{JSON.stringify(data.trace.output, null, 2)}</pre>
					</div>
				{/if}
			</div>
		{/if}
	{/if}
</div>
