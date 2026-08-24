<script lang="ts">
	import { api, type Dataset } from '$lib/api';
	import { currentProject, notifications } from '$lib/stores';
	import { goto } from '$app/navigation';

	let datasets = $state<Dataset[]>([]);
	let loading = $state(true);
	let showCreate = $state(false);
	let newName = $state('');
	let newDescription = $state('');
	let creating = $state(false);

	async function loadDatasets() {
		if (!$currentProject) return;
		loading = true;
		try {
			datasets = await api.datasets.list($currentProject.id);
		} catch (e: any) {
			notifications.error(e.message || 'Failed to load datasets');
		} finally {
			loading = false;
		}
	}

	async function createDataset() {
		if (!$currentProject || !newName.trim()) return;
		creating = true;
		try {
			const ds = await api.datasets.create($currentProject.id, {
				name: newName.trim(),
				description: newDescription.trim() || undefined
			});
			showCreate = false;
			newName = '';
			newDescription = '';
			notifications.success('Dataset created');
			goto(`/datasets/${ds.id}`);
		} catch (e: any) {
			notifications.error(e.message || 'Failed to create dataset');
		} finally {
			creating = false;
		}
	}

	async function deleteDataset(id: string) {
		if (!confirm('Are you sure you want to delete this dataset?')) return;
		try {
			await api.datasets.delete(id);
			await loadDatasets();
			notifications.success('Dataset deleted');
		} catch (e: any) {
			notifications.error(e.message || 'Failed to delete dataset');
		}
	}

	$effect(() => {
		if ($currentProject) {
			loadDatasets();
		}
	});
</script>

<svelte:head>
	<title>Datasets — Langfuse Light</title>
</svelte:head>

<div class="max-w-7xl mx-auto">
	<div class="flex items-center justify-between mb-6">
		<h1 class="text-2xl font-bold text-gray-900">Datasets</h1>
		<button
			onclick={() => showCreate = true}
			class="px-4 py-2 bg-blue-600 text-white rounded-md text-sm font-medium hover:bg-blue-700"
		>
			New Dataset
		</button>
	</div>

	{#if showCreate}
		<div class="bg-white rounded-lg border border-gray-200 p-6 mb-6">
			<h2 class="text-lg font-semibold text-gray-900 mb-4">Create Dataset</h2>
			<div class="space-y-4">
				<div>
					<label for="ds-name" class="block text-sm font-medium text-gray-700">Name</label>
					<input
						id="ds-name"
						type="text"
						bind:value={newName}
						placeholder="e.g. test-set-v1"
						class="mt-1 block w-full px-3 py-2 border border-gray-300 rounded-md shadow-sm focus:outline-none focus:ring-2 focus:ring-blue-500 text-sm"
					/>
				</div>
				<div>
					<label for="ds-desc" class="block text-sm font-medium text-gray-700">Description (optional)</label>
					<textarea
						id="ds-desc"
						bind:value={newDescription}
						rows="2"
						class="mt-1 block w-full px-3 py-2 border border-gray-300 rounded-md shadow-sm focus:outline-none focus:ring-2 focus:ring-blue-500 text-sm"
					></textarea>
				</div>
				<div class="flex gap-3">
					<button
						onclick={createDataset}
						disabled={creating || !newName.trim()}
						class="px-4 py-2 bg-blue-600 text-white rounded-md text-sm font-medium hover:bg-blue-700 disabled:opacity-50"
					>
						{creating ? 'Creating...' : 'Create'}
					</button>
					<button
						onclick={() => { showCreate = false; newName = ''; newDescription = ''; }}
						class="px-4 py-2 border border-gray-300 text-gray-700 rounded-md text-sm font-medium hover:bg-gray-50"
					>
						Cancel
					</button>
				</div>
			</div>
		</div>
	{/if}

	{#if loading}
		<div class="text-center py-12 text-gray-500">Loading datasets...</div>
	{:else if datasets.length === 0}
		<div class="text-center py-12 bg-white rounded-lg border border-gray-200">
			<svg class="mx-auto h-12 w-12 text-gray-400" fill="none" stroke="currentColor" viewBox="0 0 24 24">
				<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 7v10c0 2.21 3.582 4 8 4s8-1.79 8-4V7M4 7c0 2.21 3.582 4 8 4s8-1.79 8-4M4 7c0-2.21 3.582-4 8-4s8 1.79 8 4" />
			</svg>
			<h3 class="mt-2 text-sm font-medium text-gray-900">No datasets</h3>
			<p class="mt-1 text-sm text-gray-500">Create a dataset to get started with batch evaluations.</p>
		</div>
	{:else}
		<div class="bg-white rounded-lg border border-gray-200 overflow-hidden">
			<table class="min-w-full divide-y divide-gray-200">
				<thead class="bg-gray-50">
					<tr>
						<th class="px-4 py-3 text-left text-xs font-medium text-gray-500 uppercase">Name</th>
						<th class="px-4 py-3 text-left text-xs font-medium text-gray-500 uppercase">Description</th>
						<th class="px-4 py-3 text-left text-xs font-medium text-gray-500 uppercase">Created</th>
						<th class="px-4 py-3 text-right text-xs font-medium text-gray-500 uppercase">Actions</th>
					</tr>
				</thead>
				<tbody class="divide-y divide-gray-200">
					{#each datasets as ds}
						<tr class="hover:bg-gray-50">
							<td class="px-4 py-3">
								<a href="/datasets/{ds.id}" class="text-sm font-medium text-blue-600 hover:underline">
									{ds.name}
								</a>
							</td>
							<td class="px-4 py-3 text-sm text-gray-600 max-w-xs truncate">{ds.description || '—'}</td>
							<td class="px-4 py-3 text-sm text-gray-600">{new Date(ds.created_at).toLocaleDateString()}</td>
							<td class="px-4 py-3 text-right">
								<button onclick={() => deleteDataset(ds.id)} class="text-sm text-red-600 hover:text-red-800">Delete</button>
							</td>
						</tr>
					{/each}
				</tbody>
			</table>
		</div>
	{/if}
</div>
