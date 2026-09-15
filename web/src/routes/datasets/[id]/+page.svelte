<script lang="ts">
	import { t } from '$lib/i18n';
	import { api, type Dataset, type DatasetItem, type DatasetRun, type DatasetRunItem } from '$lib/api';
	import { notifications } from '$lib/stores';
	import { page } from '$app/state';

	let dataset = $state<Dataset | null>(null);
	let items = $state<DatasetItem[]>([]);
	let runs = $state<DatasetRun[]>([]);
	let loading = $state(true);
	let activeTab = $state<'items' | 'runs'>('items');

	let showAddItem = $state(false);
	let itemInput = $state('');
	let itemExpected = $state('');
	let addingItem = $state(false);

	let showCreateRun = $state(false);
	let runName = $state('');
	let creatingRun = $state(false);

	let selectedRun = $state<DatasetRun | null>(null);
	let runItems = $state<DatasetRunItem[]>([]);
	let loadingRunItems = $state(false);

	let showImport = $state(false);
	let importData = $state('');
	let importing = $state(false);

	const datasetId = $derived(page.params.id);

	async function loadDataset() {
		if (!datasetId) return;
		loading = true;
		try {
			const [ds, itemList, runList] = await Promise.all([
				api.datasets.get(datasetId),
				api.datasets.items.list(datasetId),
				api.datasets.runs.list(datasetId)
			]);
			dataset = ds;
			items = itemList;
			runs = runList;
		} catch (e: any) {
			notifications.error(e.message || 'Failed to load dataset');
		} finally {
			loading = false;
		}
	}

	async function addItem() {
		if (!datasetId || !itemInput.trim()) return;
		addingItem = true;
		try {
			const input = JSON.parse(itemInput);
			const expected = itemExpected.trim() ? JSON.parse(itemExpected) : undefined;
			await api.datasets.items.create(datasetId, { input, expected_output: expected });
			showAddItem = false;
			itemInput = '';
			itemExpected = '';
			items = await api.datasets.items.list(datasetId);
			notifications.success('Item added');
		} catch (e: any) {
			notifications.error(e.message || 'Failed to add item');
		} finally {
			addingItem = false;
		}
	}

	async function importItems() {
		if (!datasetId || !importData.trim()) return;
		importing = true;
		try {
			const parsed = JSON.parse(importData);
			await api.datasets.importItems(datasetId, Array.isArray(parsed) ? parsed : [parsed], 'json');
			showImport = false;
			importData = '';
			items = await api.datasets.items.list(datasetId);
			notifications.success('Items imported');
		} catch (e: any) {
			notifications.error(e.message || 'Failed to import items');
		} finally {
			importing = false;
		}
	}

	async function exportItems() {
		if (!datasetId) return;
		try {
			const data = await api.datasets.exportItems(datasetId, 'json');
			const blob = new Blob([JSON.stringify(data, null, 2)], { type: 'application/json' });
			const url = URL.createObjectURL(blob);
			const a = document.createElement('a');
			a.href = url;
			a.download = `${dataset?.name || 'dataset'}_items.json`;
			a.click();
			URL.revokeObjectURL(url);
			notifications.success('Exported');
		} catch (e: any) {
			notifications.error(e.message || 'Failed to export');
		}
	}

	async function createRun() {
		if (!datasetId || !runName.trim()) return;
		creatingRun = true;
		try {
			await api.datasets.runs.create(datasetId, { name: runName.trim() });
			showCreateRun = false;
			runName = '';
			runs = await api.datasets.runs.list(datasetId);
			notifications.success('Run created');
		} catch (e: any) {
			notifications.error(e.message || 'Failed to create run');
		} finally {
			creatingRun = false;
		}
	}

	async function selectRun(run: DatasetRun) {
		selectedRun = run;
		loadingRunItems = true;
		try {
			runItems = await api.datasets.runs.items.list(datasetId!, run.id);
		} catch (e: any) {
			notifications.error(e.message || 'Failed to load run items');
		} finally {
			loadingRunItems = false;
		}
	}

	async function exportRun(runId: string) {
		if (!datasetId) return;
		try {
			const data = await api.datasets.runs.export(datasetId, runId, 'json');
			const blob = new Blob([JSON.stringify(data, null, 2)], { type: 'application/json' });
			const url = URL.createObjectURL(blob);
			const a = document.createElement('a');
			a.href = url;
			a.download = `run_${runId.slice(0, 8)}.json`;
			a.click();
			URL.revokeObjectURL(url);
		} catch (e: any) {
			notifications.error(e.message || 'Failed to export run');
		}
	}

	function formatJson(obj: unknown): string {
		if (obj == null) return '—';
		if (typeof obj === 'string') return obj;
		return JSON.stringify(obj, null, 2);
	}

	$effect(() => {
		if (datasetId) {
			loadDataset();
		}
	});
</script>

<svelte:head>
	<title>{dataset?.name || $t('datasets.detail.loadingDataset')} — {$t('app.name')}</title>
</svelte:head>

<div class="max-w-7xl mx-auto">
	{#if loading}
		<div class="text-center py-12 text-gray-500">{$t('datasets.detail.loadingDataset')}</div>
	{:else if !dataset}
		<div class="text-center py-12 text-gray-500">{$t('datasets.detail.notFound')}</div>
	{:else}
		<!-- Header -->
		<div class="mb-6">
			<div class="flex items-center gap-2 text-sm text-gray-500 mb-2">
				<a href="/datasets" class="hover:text-gray-700">{$t('datasets.heading')}</a>
				<span>/</span>
				<span class="text-gray-900 font-medium">{dataset.name}</span>
			</div>
			{#if dataset.description}
				<p class="text-sm text-gray-600">{dataset.description}</p>
			{/if}
		</div>

		<!-- Tabs -->
		<div class="flex gap-4 border-b border-gray-200 mb-6">
			<button
				onclick={() => activeTab = 'items'}
				class="pb-3 text-sm font-medium border-b-2 {activeTab === 'items' ? 'border-blue-500 text-blue-600' : 'border-transparent text-gray-500 hover:text-gray-700'}"
			>
				{$t('datasets.detail.items')} ({items.length})
			</button>
			<button
				onclick={() => { activeTab = 'runs'; selectedRun = null; }}
				class="pb-3 text-sm font-medium border-b-2 {activeTab === 'runs' ? 'border-blue-500 text-blue-600' : 'border-transparent text-gray-500 hover:text-gray-700'}"
			>
				{$t('datasets.detail.runs')} ({runs.length})
			</button>
		</div>

		<!-- Items Tab -->
		{#if activeTab === 'items'}
			<div class="flex items-center justify-between mb-4">
				<span class="text-sm text-gray-500">{items.length} {$t('datasets.detail.items')}</span>
				<div class="flex gap-2">
					<button
						onclick={() => showImport = true}
						class="px-3 py-1.5 text-sm border border-gray-300 rounded-md hover:bg-gray-50"
					>
						{$t('datasets.detail.importJson')}
					</button>
					<button
						onclick={exportItems}
						class="px-3 py-1.5 text-sm border border-gray-300 rounded-md hover:bg-gray-50"
					>
						{$t('common.export')}
					</button>
					<button
						onclick={() => showAddItem = true}
						class="px-3 py-1.5 text-sm bg-blue-600 text-white rounded-md hover:bg-blue-700"
					>
						{$t('datasets.detail.addItem')}
					</button>
				</div>
			</div>

			{#if showImport}
				<div class="bg-white rounded-lg border border-gray-200 p-4 mb-4">
					<h3 class="text-sm font-medium text-gray-900 mb-2">{$t('datasets.detail.importItems')}</h3>
					<textarea
						bind:value={importData}
						rows="4"
						placeholder={$t('datasets.detail.importPlaceholder')}
						class="w-full px-3 py-2 border border-gray-300 rounded-md text-sm font-mono focus:outline-none focus:ring-2 focus:ring-blue-500"
					></textarea>
					<div class="flex gap-2 mt-3">
						<button onclick={importItems} disabled={importing} class="px-3 py-1.5 text-sm bg-blue-600 text-white rounded-md hover:bg-blue-700 disabled:opacity-50">
							{importing ? $t('common.importing') : $t('common.import')}
						</button>
						<button onclick={() => { showImport = false; importData = ''; }} class="px-3 py-1.5 text-sm border border-gray-300 rounded-md hover:bg-gray-50">{$t('common.cancel')}</button>
					</div>
				</div>
			{/if}

			{#if showAddItem}
				<div class="bg-white rounded-lg border border-gray-200 p-4 mb-4">
					<h3 class="text-sm font-medium text-gray-900 mb-2">{$t('datasets.detail.addItem')}</h3>
					<div class="space-y-3">
						<div>
							<label for="item-input" class="block text-xs text-gray-500 mb-1">{$t('datasets.detail.itemInput')}</label>
							<textarea
								id="item-input"
								bind:value={itemInput}
								rows="3"
								class="w-full px-3 py-2 border border-gray-300 rounded-md text-sm font-mono focus:outline-none focus:ring-2 focus:ring-blue-500"
							></textarea>
						</div>
						<div>
							<label for="item-expected" class="block text-xs text-gray-500 mb-1">{$t('datasets.detail.itemExpected')}</label>
							<textarea
								id="item-expected"
								bind:value={itemExpected}
								rows="3"
								class="w-full px-3 py-2 border border-gray-300 rounded-md text-sm font-mono focus:outline-none focus:ring-2 focus:ring-blue-500"
							></textarea>
						</div>
						<div class="flex gap-2">
							<button onclick={addItem} disabled={addingItem || !itemInput.trim()} class="px-3 py-1.5 text-sm bg-blue-600 text-white rounded-md hover:bg-blue-700 disabled:opacity-50">
								{addingItem ? $t('common.adding') : $t('common.add')}
							</button>
							<button onclick={() => { showAddItem = false; itemInput = ''; itemExpected = ''; }} class="px-3 py-1.5 text-sm border border-gray-300 rounded-md hover:bg-gray-50">{$t('common.cancel')}</button>
						</div>
					</div>
				</div>
			{/if}

			{#if items.length === 0}
				<div class="text-center py-12 bg-white rounded-lg border border-gray-200">
					<p class="text-sm text-gray-500">{$t('datasets.detail.noItems')}</p>
				</div>
			{:else}
				<div class="space-y-3">
					{#each items as item}
						<div class="bg-white rounded-lg border border-gray-200 p-4">
							<div class="flex items-start justify-between">
								<div class="flex-1 min-w-0">
									<div class="text-xs text-gray-400 mb-1">{item.id.slice(0, 8)}</div>
									<div class="grid grid-cols-2 gap-4">
										<div>
											<div class="text-xs font-medium text-gray-500 mb-1">{$t('datasets.detail.input')}</div>
											<pre class="text-xs bg-gray-50 rounded p-2 overflow-x-auto whitespace-pre-wrap">{formatJson(item.input)}</pre>
										</div>
										<div>
											<div class="text-xs font-medium text-gray-500 mb-1">{$t('datasets.detail.expectedOutput')}</div>
											<pre class="text-xs bg-gray-50 rounded p-2 overflow-x-auto whitespace-pre-wrap">{formatJson(item.expected_output)}</pre>
										</div>
									</div>
								</div>
							</div>
						</div>
					{/each}
				</div>
			{/if}
		{/if}

		<!-- Runs Tab -->
		{#if activeTab === 'runs'}
			<div class="flex items-center justify-between mb-4">
				<span class="text-sm text-gray-500">{runs.length} {$t('datasets.detail.runs')}</span>
				<button
					onclick={() => showCreateRun = true}
					class="px-3 py-1.5 text-sm bg-blue-600 text-white rounded-md hover:bg-blue-700"
				>
					{$t('datasets.detail.newRun')}
				</button>
			</div>

			{#if showCreateRun}
				<div class="bg-white rounded-lg border border-gray-200 p-4 mb-4">
					<h3 class="text-sm font-medium text-gray-900 mb-2">{$t('datasets.detail.createRun')}</h3>
					<div class="flex gap-3">
						<input
							type="text"
							bind:value={runName}
							placeholder={$t('datasets.detail.runPlaceholder')}
							class="flex-1 px-3 py-2 border border-gray-300 rounded-md text-sm focus:outline-none focus:ring-2 focus:ring-blue-500"
							onkeydown={(e) => { if (e.key === 'Enter') createRun(); }}
						/>
						<button onclick={createRun} disabled={creatingRun || !runName.trim()} class="px-3 py-2 bg-blue-600 text-white rounded-md text-sm hover:bg-blue-700 disabled:opacity-50">
							{creatingRun ? $t('common.creating') : $t('common.create')}
						</button>
						<button onclick={() => { showCreateRun = false; runName = ''; }} class="px-3 py-2 border border-gray-300 rounded-md text-sm hover:bg-gray-50">{$t('common.cancel')}</button>
					</div>
				</div>
			{/if}

			{#if runs.length === 0}
				<div class="text-center py-12 bg-white rounded-lg border border-gray-200">
					<p class="text-sm text-gray-500">{$t('datasets.detail.noRuns')}</p>
				</div>
			{:else}
				<div class="bg-white rounded-lg border border-gray-200 overflow-hidden">
					<table class="min-w-full divide-y divide-gray-200">
						<thead class="bg-gray-50">
							<tr>
								<th class="px-4 py-3 text-left text-xs font-medium text-gray-500 uppercase">{$t('common.name')}</th>
								<th class="px-4 py-3 text-left text-xs font-medium text-gray-500 uppercase">{$t('common.created')}</th>
								<th class="px-4 py-3 text-right text-xs font-medium text-gray-500 uppercase">{$t('common.actions')}</th>
							</tr>
						</thead>
						<tbody class="divide-y divide-gray-200">
							{#each runs as run}
								<tr class="hover:bg-gray-50 {selectedRun?.id === run.id ? 'bg-blue-50' : ''}">
									<td class="px-4 py-3">
										<button onclick={() => selectRun(run)} class="text-sm font-medium text-blue-600 hover:underline">
											{run.name}
										</button>
									</td>
									<td class="px-4 py-3 text-sm text-gray-600">{new Date(run.created_at).toLocaleDateString()}</td>
									<td class="px-4 py-3 text-right">
										<button onclick={() => exportRun(run.id)} class="text-sm text-blue-600 hover:text-blue-800">{$t('common.export')}</button>
									</td>
								</tr>
							{/each}
						</tbody>
					</table>
				</div>

				{#if selectedRun}
					<div class="mt-6 bg-white rounded-lg border border-gray-200 p-4">
						<h3 class="text-sm font-medium text-gray-900 mb-3">{$t('datasets.detail.runLabel')} {selectedRun.name}</h3>
						{#if loadingRunItems}
							<div class="text-sm text-gray-500">{$t('datasets.detail.loadingRunItems')}</div>
						{:else if runItems.length === 0}
							<div class="text-sm text-gray-500">{$t('datasets.detail.noRunItems')}</div>
						{:else}
							<div class="text-xs text-gray-500 mb-2">{runItems.length} {$t('datasets.detail.items')}</div>
							<div class="overflow-x-auto">
								<table class="min-w-full text-xs">
									<thead>
										<tr class="border-b">
											<th class="px-3 py-2 text-left font-medium text-gray-500">{$t('datasets.detail.itemId')}</th>
											<th class="px-3 py-2 text-left font-medium text-gray-500">{$t('datasets.detail.observation')}</th>
											<th class="px-3 py-2 text-left font-medium text-gray-500">{$t('datasets.detail.score')}</th>
										</tr>
									</thead>
									<tbody>
										{#each runItems as ri}
											<tr class="border-b">
												<td class="px-3 py-2 font-mono">{ri.dataset_item_id.slice(0, 8)}</td>
												<td class="px-3 py-2 font-mono">{ri.observation_id ? ri.observation_id.slice(0, 8) : '—'}</td>
												<td class="px-3 py-2 font-mono">{ri.score_id ? ri.score_id.slice(0, 8) : '—'}</td>
											</tr>
										{/each}
									</tbody>
								</table>
							</div>
						{/if}
					</div>
				{/if}
			{/if}
		{/if}
	{/if}
</div>
