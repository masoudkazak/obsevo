<script lang="ts">
	import { t } from '$lib/i18n';
	import { api, type Prompt } from '$lib/api';
	import { currentProject, notifications } from '$lib/stores';
	import { goto } from '$app/navigation';

	let prompts = $state<Prompt[]>([]);
	let loading = $state(true);
	let searchQuery = $state('');
	let showCreateModal = $state(false);
	let newName = $state('');
	let newPrompt = $state('');
	let creating = $state(false);

	async function loadPrompts() {
		if (!$currentProject) return;
		loading = true;
		try {
			const res = await api.prompts.list($currentProject.id);
			prompts = res.prompts;
		} catch (e: any) {
			notifications.error(e.message || 'Failed to load prompts');
		} finally {
			loading = false;
		}
	}

	$effect(() => {
		if ($currentProject) {
			loadPrompts();
		} else {
			loading = false;
		}
	});

	function groupByName(list: Prompt[]): Map<string, Prompt[]> {
		const map = new Map<string, Prompt[]>();
		for (const p of list) {
			const existing = map.get(p.name) || [];
			existing.push(p);
			map.set(p.name, existing);
		}
		return map;
	}

	let grouped = $derived(
		groupByName(
			searchQuery
				? prompts.filter(p => p.name.toLowerCase().includes(searchQuery.toLowerCase()))
				: prompts
		)
	);

	function formatTime(ts: string): string {
		return new Date(ts).toLocaleString();
	}

	async function createPrompt() {
		if (!$currentProject || !newName.trim() || !newPrompt.trim()) return;
		creating = true;
		try {
			await api.prompts.create({ name: newName.trim(), prompt: newPrompt.trim() }, $currentProject.id);
			notifications.success($t('prompts.promptCreated'));
			showCreateModal = false;
			newName = '';
			newPrompt = '';
			await loadPrompts();
		} catch (e: any) {
			notifications.error(e.message || 'Failed to create prompt');
		} finally {
			creating = false;
		}
	}
</script>

<svelte:head>
	<title>{$t('prompts.title')}</title>
</svelte:head>

<div class="max-w-7xl mx-auto">
	<div class="flex items-center justify-between mb-6">
		<h1 class="text-2xl font-bold text-gray-900">{$t('prompts.heading')}</h1>
		<button
			onclick={() => showCreateModal = true}
			class="px-4 py-2 bg-blue-600 text-white rounded-md text-sm font-medium hover:bg-blue-700"
		>
			{$t('prompts.newPrompt')}
		</button>
	</div>

	<!-- Search -->
	<div class="mb-4">
		<input
			type="text"
			bind:value={searchQuery}
			placeholder={$t('prompts.searchPlaceholder')}
			class="px-3 py-2 border border-gray-300 rounded-md shadow-sm focus:outline-none focus:ring-2 focus:ring-blue-500 focus:border-blue-500 text-sm w-64"
		/>
	</div>

	{#if loading}
		<div class="text-center py-12 text-gray-500">{$t('prompts.loadingPrompts')}</div>
	{:else if !$currentProject}
		<div class="text-center py-12 bg-white rounded-lg border border-gray-200">
			<h3 class="text-sm font-medium text-gray-900">{$t('common.noProject')}</h3>
			<p class="mt-1 text-sm text-gray-500">{$t('common.noProjectDesc')}</p>
		</div>
	{:else if grouped.size === 0}
		<div class="text-center py-12 bg-white rounded-lg border border-gray-200">
			<svg class="mx-auto h-12 w-12 text-gray-400" fill="none" stroke="currentColor" viewBox="0 0 24 24">
				<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M11 5H6a2 2 0 00-2 2v11a2 2 0 002 2h11a2 2 0 002-2v-5m-1.414-9.414a2 2 0 112.828 2.828L11.828 15H9v-2.828l8.586-8.586z" />
			</svg>
			<h3 class="mt-2 text-sm font-medium text-gray-900">{$t('prompts.noPrompts')}</h3>
			<p class="mt-1 text-sm text-gray-500">{$t('prompts.noPromptsDesc')}</p>
		</div>
	{:else}
		<div class="space-y-3">
			{#each [...grouped.entries()] as [name, versions]}
				{@const active = versions.find(v => v.is_active) || versions[0]}
				{@const versionCount = versions.length}
				<button
					class="w-full text-left bg-white rounded-lg border border-gray-200 p-4 hover:border-blue-300 transition-colors"
					onclick={() => goto(`/prompts/${encodeURIComponent(name)}`)}
				>
					<div class="flex items-center justify-between">
						<div>
							<h3 class="text-sm font-semibold text-gray-900">{name}</h3>
							<p class="text-xs text-gray-500 mt-1 line-clamp-1">{active.prompt.slice(0, 120)}</p>
						</div>
						<div class="text-right shrink-0 ml-4">
							<span class="inline-block px-2 py-0.5 bg-blue-50 text-blue-700 rounded text-xs font-medium">
								v{active.version}
							</span>
							{#if versionCount > 1}
								<span class="text-xs text-gray-400 ml-2">{versionCount} {$t('prompts.versionsCount')}</span>
							{/if}
							<p class="text-xs text-gray-400 mt-1">{formatTime(active.created_at)}</p>
						</div>
					</div>
				</button>
			{/each}
		</div>
	{/if}
</div>

<!-- Create modal -->
{#if showCreateModal}
	<div class="fixed inset-0 bg-black/50 flex items-center justify-center z-50">
		<div class="bg-white rounded-lg shadow-xl w-full max-w-lg mx-4 p-6">
			<h2 class="text-lg font-semibold mb-4">{$t('prompts.createPrompt')}</h2>
			<form onsubmit={(e) => { e.preventDefault(); createPrompt(); }}>
				<div class="mb-4">
					<label for="pname" class="block text-sm font-medium text-gray-700 mb-1">{$t('common.name')}</label>
					<input
						id="pname"
						type="text"
						bind:value={newName}
						required
						class="w-full px-3 py-2 border border-gray-300 rounded-md shadow-sm focus:outline-none focus:ring-2 focus:ring-blue-500 text-sm"
						placeholder={$t('prompts.namePlaceholder')}
					/>
				</div>
				<div class="mb-6">
					<label for="pprompt" class="block text-sm font-medium text-gray-700 mb-1">{$t('prompts.promptTemplate')}</label>
					<textarea
						id="pprompt"
						bind:value={newPrompt}
						required
						rows="6"
						class="w-full px-3 py-2 border border-gray-300 rounded-md shadow-sm focus:outline-none focus:ring-2 focus:ring-blue-500 text-sm font-mono"
						placeholder={'Summarize the following text: {{input}}'}
					></textarea>
				</div>
				<div class="flex justify-end gap-3">
					<button
						type="button"
						onclick={() => showCreateModal = false}
						class="px-4 py-2 text-sm text-gray-700 hover:bg-gray-100 rounded-md"
					>
						{$t('common.cancel')}
					</button>
					<button
						type="submit"
						disabled={creating}
						class="px-4 py-2 bg-blue-600 text-white rounded-md text-sm font-medium hover:bg-blue-700 disabled:opacity-50"
					>
						{creating ? $t('common.creating') : $t('common.create')}
					</button>
				</div>
			</form>
		</div>
	</div>
{/if}
