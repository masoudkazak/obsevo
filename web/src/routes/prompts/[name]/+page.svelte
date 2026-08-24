<script lang="ts">
	import { page } from '$app/state';
	import { api, type Prompt, type PromptWithVariables } from '$lib/api';
	import { currentProject, notifications } from '$lib/stores';

	let prompt = $state<PromptWithVariables | null>(null);
	let versions = $state<Prompt[]>([]);
	let loading = $state(true);
	let activeTab = $state<'editor' | 'versions'>('editor');
	let editContent = $state('');
	let editConfig = $state('{}');
	let saving = $state(false);
	let showNewVersion = $state(false);
	let newContent = $state('');
	let newConfig = $state('{}');
	let previewVars = $state<Record<string, string>>({});

	$effect(() => {
		const name = page.params.name;
		if (name && $currentProject) {
			loadPrompt(decodeURIComponent(name));
		}
	});

	async function loadPrompt(name: string) {
		loading = true;
		try {
			const [p, v] = await Promise.all([
				api.prompts.getByName(name, $currentProject!.id),
				api.prompts.versions(name, $currentProject!.id)
			]);
			prompt = p;
			versions = v.versions;
			editContent = p.prompt;
			editConfig = JSON.stringify(p.config || {}, null, 2);
		} catch (e: any) {
			notifications.error(e.message || 'Failed to load prompt');
		} finally {
			loading = false;
		}
	}

	function formatTime(ts: string): string {
		return new Date(ts).toLocaleString();
	}

	function extractVariables(template: string): string[] {
		const matches = template.match(/\{\{(\w+)\}\}/g);
		if (!matches) return [];
		return [...new Set(matches.map(m => m.replace(/[{}]/g, '')))];
	}

	let detectedVars = $derived(editContent ? extractVariables(editContent) : []);

	let compiledPreview = $derived.by(() => {
		if (!editContent) return '';
		let result = editContent;
		for (const [key, value] of Object.entries(previewVars)) {
			result = result.replaceAll(`{{${key}}}`, value || `{{${key}}}`);
		}
		return result;
	});

	async function savePrompt() {
		if (!prompt || !$currentProject) return;
		saving = true;
		try {
			let config: unknown = {};
			try { config = JSON.parse(editConfig); } catch {}
			await api.prompts.update(
				prompt.name,
				{ prompt: editContent, config },
				$currentProject.id
			);
			notifications.success('Prompt updated');
			await loadPrompt(prompt.name);
		} catch (e: any) {
			notifications.error(e.message || 'Failed to update prompt');
		} finally {
			saving = false;
		}
	}

	async function createNewVersion() {
		if (!prompt || !$currentProject || !newContent.trim()) return;
		saving = true;
		try {
			let config: unknown = {};
			try { config = JSON.parse(newConfig); } catch {}
			await api.prompts.update(
				prompt.name,
				{ prompt: newContent.trim(), config, is_active: true },
				$currentProject.id
			);
			notifications.success('New version created');
			showNewVersion = false;
			newContent = '';
			newConfig = '{}';
			await loadPrompt(prompt.name);
		} catch (e: any) {
			notifications.error(e.message || 'Failed to create version');
		} finally {
			saving = false;
		}
	}

	async function setActiveVersion(version: number) {
		if (!prompt || !$currentProject) return;
		try {
			await api.prompts.setActive(prompt.name, version, $currentProject.id);
			notifications.success(`Version ${version} set as active`);
			await loadPrompt(prompt.name);
		} catch (e: any) {
			notifications.error(e.message || 'Failed to set active version');
		}
	}
</script>

<svelte:head>
	<title>{page.params.name} — Langfuse Light</title>
</svelte:head>

<div class="max-w-5xl mx-auto">
	<a href="/prompts" class="text-sm text-blue-600 hover:underline mb-4 inline-block">&larr; Back to prompts</a>

	{#if loading}
		<div class="text-center py-12 text-gray-500">Loading prompt...</div>
	{:else if prompt}
		<!-- Header -->
		<div class="flex items-center justify-between mb-6">
			<div>
				<h1 class="text-2xl font-bold text-gray-900">{prompt.name}</h1>
				<p class="text-sm text-gray-500 mt-1">
					Active version: <span class="font-medium text-blue-600">v{prompt.version}</span>
					&middot; {versions.length} total versions
				</p>
			</div>
			<button
				onclick={() => { showNewVersion = true; newContent = editContent; }}
				class="px-4 py-2 bg-blue-600 text-white rounded-md text-sm font-medium hover:bg-blue-700"
			>
				New Version
			</button>
		</div>

		<!-- Tabs -->
		<div class="flex border-b border-gray-200 mb-6">
			<button
				class="px-4 py-2 text-sm font-medium border-b-2 {activeTab === 'editor' ? 'border-blue-600 text-blue-600' : 'border-transparent text-gray-500 hover:text-gray-700'}"
				onclick={() => activeTab = 'editor'}
			>
				Editor
			</button>
			<button
				class="px-4 py-2 text-sm font-medium border-b-2 {activeTab === 'versions' ? 'border-blue-600 text-blue-600' : 'border-transparent text-gray-500 hover:text-gray-700'}"
				onclick={() => activeTab = 'versions'}
			>
				Versions
			</button>
		</div>

		{#if activeTab === 'editor'}
			<!-- Variables detected -->
			{#if detectedVars.length > 0}
				<div class="mb-4 p-3 bg-blue-50 border border-blue-200 rounded-md">
					<span class="text-xs font-medium text-blue-700">Template variables: </span>
					{#each detectedVars as v}
						<code class="px-1.5 py-0.5 bg-blue-100 text-blue-800 rounded text-xs mr-1">{'{{'}{v}{'}}'}</code>
					{/each}
				</div>
			{/if}

			<!-- Editor -->
			<div class="bg-white rounded-lg border border-gray-200 p-6">
				<div class="mb-4">
					<label for="edit-prompt" class="block text-sm font-medium text-gray-700 mb-1">Prompt Template</label>
					<textarea
						id="edit-prompt"
						bind:value={editContent}
						rows="12"
						class="w-full px-3 py-2 border border-gray-300 rounded-md shadow-sm focus:outline-none focus:ring-2 focus:ring-blue-500 text-sm font-mono"
					></textarea>
				</div>
				<div class="mb-4">
					<label for="edit-config" class="block text-sm font-medium text-gray-700 mb-1">Config (JSON)</label>
					<textarea
						id="edit-config"
						bind:value={editConfig}
						rows="3"
						class="w-full px-3 py-2 border border-gray-300 rounded-md shadow-sm focus:outline-none focus:ring-2 focus:ring-blue-500 text-sm font-mono"
					></textarea>
				</div>
				<button
					onclick={savePrompt}
					disabled={saving}
					class="px-4 py-2 bg-blue-600 text-white rounded-md text-sm font-medium hover:bg-blue-700 disabled:opacity-50"
				>
					{saving ? 'Saving...' : 'Save Changes'}
				</button>
			</div>

			<!-- Preview -->
			<div class="bg-white rounded-lg border border-gray-200 p-6 mt-6">
				<h3 class="text-sm font-medium text-gray-700 mb-3">Template Preview</h3>
				{#if detectedVars.length > 0}
					<div class="mb-4 space-y-2">
						{#each detectedVars as v}
							<div class="flex items-center gap-2">
								<label for="preview-{v}" class="text-xs font-mono text-gray-500 w-32 shrink-0">{'{{'}{v}{'}}'}</label>
								<input
									id="preview-{v}"
									type="text"
									bind:value={previewVars[v]}
									placeholder={`value for ${v}`}
									class="flex-1 px-2 py-1 border border-gray-200 rounded text-sm font-mono focus:outline-none focus:ring-1 focus:ring-blue-500"
								/>
							</div>
						{/each}
					</div>
				{/if}
				<div class="p-4 bg-gray-50 rounded text-sm font-mono whitespace-pre-wrap">{compiledPreview}</div>
			</div>
		{:else}
			<!-- Versions list -->
			<div class="space-y-3">
				{#each versions as v}
					<div class="bg-white rounded-lg border border-gray-200 p-4 flex items-center justify-between">
						<div>
							<div class="flex items-center gap-2">
								<span class="text-sm font-semibold text-gray-900">Version {v.version}</span>
								{#if v.is_active}
									<span class="px-2 py-0.5 bg-green-100 text-green-700 rounded text-xs font-medium">Active</span>
								{/if}
							</div>
							<p class="text-xs text-gray-500 mt-1">{formatTime(v.created_at)}</p>
							<p class="text-xs text-gray-400 mt-1 line-clamp-1 font-mono">{v.prompt.slice(0, 100)}</p>
						</div>
						{#if !v.is_active}
							<button
								onclick={() => setActiveVersion(v.version)}
								class="px-3 py-1.5 text-xs border border-gray-300 rounded-md hover:bg-gray-50"
							>
								Set Active
							</button>
						{/if}
					</div>
				{/each}
			</div>
		{/if}
	{/if}
</div>

<!-- New version modal -->
{#if showNewVersion}
	<div class="fixed inset-0 bg-black/50 flex items-center justify-center z-50">
		<div class="bg-white rounded-lg shadow-xl w-full max-w-lg mx-4 p-6">
			<h2 class="text-lg font-semibold mb-4">New Version</h2>
			<form onsubmit={(e) => { e.preventDefault(); createNewVersion(); }}>
				<div class="mb-4">
					<label for="new-prompt" class="block text-sm font-medium text-gray-700 mb-1">Prompt Template</label>
					<textarea
						id="new-prompt"
						bind:value={newContent}
						rows="8"
						required
						class="w-full px-3 py-2 border border-gray-300 rounded-md shadow-sm focus:outline-none focus:ring-2 focus:ring-blue-500 text-sm font-mono"
					></textarea>
				</div>
				<div class="mb-6">
					<label for="new-config" class="block text-sm font-medium text-gray-700 mb-1">Config (JSON)</label>
					<textarea
						id="new-config"
						bind:value={newConfig}
						rows="3"
						class="w-full px-3 py-2 border border-gray-300 rounded-md shadow-sm focus:outline-none focus:ring-2 focus:ring-blue-500 text-sm font-mono"
					></textarea>
				</div>
				<div class="flex justify-end gap-3">
					<button
						type="button"
						onclick={() => showNewVersion = false}
						class="px-4 py-2 text-sm text-gray-700 hover:bg-gray-100 rounded-md"
					>
						Cancel
					</button>
					<button
						type="submit"
						disabled={saving}
						class="px-4 py-2 bg-blue-600 text-white rounded-md text-sm font-medium hover:bg-blue-700 disabled:opacity-50"
					>
						{saving ? 'Creating...' : 'Create Version'}
					</button>
				</div>
			</form>
		</div>
	</div>
{/if}
