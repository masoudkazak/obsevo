<script lang="ts">
	import { api, type Trace } from '$lib/api';
	import { currentProject, notifications } from '$lib/stores';
	import { goto } from '$app/navigation';

	let traces = $state<Trace[]>([]);
	let total = $state(0);
	let loading = $state(true);
	let nameFilter = $state('');
	let limit = $state(50);
	let offset = $state(0);

	async function loadTraces() {
		if (!$currentProject) return;
		loading = true;
		try {
			const res = await api.traces.list($currentProject.id, {
				name: nameFilter || undefined,
				limit,
				offset
			});
			traces = res.traces;
			total = res.total;
		} catch (e: any) {
			notifications.error(e.message || 'Failed to load traces');
		} finally {
			loading = false;
		}
	}

	$effect(() => {
		if ($currentProject) {
			loadTraces();
		} else {
			loading = false;
		}
	});

	function formatTime(ts: string | null): string {
		if (!ts) return '—';
		return new Date(ts).toLocaleString();
	}

	function formatCost(cost: number | null): string {
		if (cost == null) return '—';
		return `$${cost.toFixed(4)}`;
	}

	function formatDuration(start: string | null, end: string | null): string {
		if (!start || !end) return '—';
		const ms = new Date(end).getTime() - new Date(start).getTime();
		if (ms < 1000) return `${ms}ms`;
		return `${(ms / 1000).toFixed(2)}s`;
	}

	function nextPage() {
		offset += limit;
		loadTraces();
	}

	function prevPage() {
		offset = Math.max(0, offset - limit);
		loadTraces();
	}
</script>

<svelte:head>
	<title>Traces — Langfuse Light</title>
</svelte:head>

<div class="max-w-7xl mx-auto">
	<div class="flex items-center justify-between mb-6">
		<h1 class="text-2xl font-bold text-gray-900">Traces</h1>
		<span class="text-sm text-gray-500">{total} total</span>
	</div>

	<!-- Filters -->
	<div class="mb-4 flex gap-3">
		<input
			type="text"
			bind:value={nameFilter}
			placeholder="Filter by name..."
			class="px-3 py-2 border border-gray-300 rounded-md shadow-sm focus:outline-none focus:ring-2 focus:ring-blue-500 focus:border-blue-500 text-sm w-64"
			onkeydown={(e) => { if (e.key === 'Enter') { offset = 0; loadTraces(); } }}
		/>
		<button
			onclick={() => { offset = 0; loadTraces(); }}
			class="px-4 py-2 bg-gray-100 text-gray-700 rounded-md text-sm font-medium hover:bg-gray-200"
		>
			Search
		</button>
	</div>

	{#if loading}
		<div class="text-center py-12 text-gray-500">Loading traces...</div>
	{:else if !$currentProject}
		<div class="text-center py-12 bg-white rounded-lg border border-gray-200">
			<h3 class="text-sm font-medium text-gray-900">No project selected</h3>
			<p class="mt-1 text-sm text-gray-500">Create a project from Settings to get started.</p>
		</div>
	{:else if traces.length === 0}
		<div class="text-center py-12 bg-white rounded-lg border border-gray-200">
			<svg class="mx-auto h-12 w-12 text-gray-400" fill="none" stroke="currentColor" viewBox="0 0 24 24">
				<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 5H7a2 2 0 00-2 2v12a2 2 0 002 2h10a2 2 0 002-2V7a2 2 0 00-2-2h-2M9 5a2 2 0 002 2h2a2 2 0 002-2M9 5a2 2 0 012-2h2a2 2 0 012 2" />
			</svg>
			<h3 class="mt-2 text-sm font-medium text-gray-900">No traces</h3>
			<p class="mt-1 text-sm text-gray-500">Get started by creating a trace via the API.</p>
		</div>
	{:else}
		<div class="bg-white rounded-lg border border-gray-200 overflow-hidden">
			<table class="min-w-full divide-y divide-gray-200">
				<thead class="bg-gray-50">
					<tr>
						<th class="px-4 py-3 text-left text-xs font-medium text-gray-500 uppercase">Name</th>
						<th class="px-4 py-3 text-left text-xs font-medium text-gray-500 uppercase">Start Time</th>
						<th class="px-4 py-3 text-left text-xs font-medium text-gray-500 uppercase">Duration</th>
						<th class="px-4 py-3 text-left text-xs font-medium text-gray-500 uppercase">Cost</th>
						<th class="px-4 py-3 text-left text-xs font-medium text-gray-500 uppercase">User</th>
						<th class="px-4 py-3 text-left text-xs font-medium text-gray-500 uppercase">Tags</th>
					</tr>
				</thead>
				<tbody class="divide-y divide-gray-200">
					{#each traces as trace}
						<tr
							class="hover:bg-gray-50 cursor-pointer"
							onclick={() => goto(`/traces/${trace.id}`)}
						>
							<td class="px-4 py-3 text-sm font-medium text-blue-600 hover:underline">
								{trace.name || trace.id.slice(0, 8)}
							</td>
							<td class="px-4 py-3 text-sm text-gray-600">{formatTime(trace.start_time)}</td>
							<td class="px-4 py-3 text-sm text-gray-600">{formatDuration(trace.start_time, trace.end_time)}</td>
							<td class="px-4 py-3 text-sm text-gray-600">{formatCost(trace.total_cost)}</td>
							<td class="px-4 py-3 text-sm text-gray-600">{trace.user_id || '—'}</td>
							<td class="px-4 py-3 text-sm">
								{#each (trace.tags || []).slice(0, 3) as tag}
									<span class="inline-block px-2 py-0.5 bg-gray-100 text-gray-600 rounded text-xs mr-1">{tag}</span>
								{/each}
							</td>
						</tr>
					{/each}
				</tbody>
			</table>
		</div>

		<!-- Pagination -->
		<div class="flex items-center justify-between mt-4">
			<span class="text-sm text-gray-500">
				Showing {offset + 1}–{Math.min(offset + limit, total)} of {total}
			</span>
			<div class="flex gap-2">
				<button
					onclick={prevPage}
					disabled={offset === 0}
					class="px-3 py-1.5 text-sm border border-gray-300 rounded-md disabled:opacity-50 hover:bg-gray-50"
				>
					Previous
				</button>
				<button
					onclick={nextPage}
					disabled={offset + limit >= total}
					class="px-3 py-1.5 text-sm border border-gray-300 rounded-md disabled:opacity-50 hover:bg-gray-50"
				>
					Next
				</button>
			</div>
		</div>
	{/if}
</div>
