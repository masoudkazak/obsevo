<script lang="ts">
	import { t } from '$lib/i18n';
	import { api, type APIKey } from '$lib/api';
	import { currentProject, notifications } from '$lib/stores';

	let keys = $state<APIKey[]>([]);
	let loading = $state(true);
	let newKeyName = $state('');
	let creating = $state(false);
	let showKey = $state<string | null>(null);
	let showPublicKey = $state<string | null>(null);

	async function loadKeys() {
		if (!$currentProject) return;
		loading = true;
		try {
			keys = await api.settings.apiKeys.list($currentProject.id);
		} catch (e: any) {
			notifications.error(e.message || 'Failed to load API keys');
		} finally {
			loading = false;
		}
	}

	async function createKey() {
		if (!$currentProject || !newKeyName.trim()) return;
		creating = true;
		try {
			const key = await api.settings.apiKeys.create($currentProject.id, { name: newKeyName.trim() });
			showKey = key.secret_key || key.key;
			showPublicKey = key.public_key || null;
			newKeyName = '';
			await loadKeys();
			notifications.success($t('settings.keyCreated'));
		} catch (e: any) {
			notifications.error(e.message || 'Failed to create API key');
		} finally {
			creating = false;
		}
	}

	async function deleteKey(keyId: string) {
		if (!$currentProject) return;
		if (!confirm($t('settings.confirmDeleteKey'))) return;
		try {
			await api.settings.apiKeys.delete(keyId, $currentProject.id);
			await loadKeys();
			notifications.success($t('settings.keyDeleted'));
		} catch (e: any) {
			notifications.error(e.message || 'Failed to delete API key');
		}
	}

	function copyKey(key: string) {
		navigator.clipboard.writeText(key);
		notifications.success($t('settings.keyCopied'));
	}

	$effect(() => {
		if ($currentProject) {
			loadKeys();
		} else {
			loading = false;
		}
	});
</script>

<svelte:head>
	<title>{$t('settings.apiKeysTitle')}</title>
</svelte:head>

<div class="bg-white rounded-lg border border-gray-200 p-6">
	<h2 class="text-lg font-semibold text-gray-900 mb-4">{$t('settings.apiKeyHeading')}</h2>

	<!-- Create new key -->
	<div class="flex gap-3 mb-6">
		<input
			type="text"
			bind:value={newKeyName}
			placeholder={$t('settings.keyNamePlaceholder')}
			class="flex-1 px-3 py-2 border border-gray-300 rounded-md shadow-sm focus:outline-none focus:ring-2 focus:ring-blue-500 text-sm"
			onkeydown={(e) => { if (e.key === 'Enter') createKey(); }}
		/>
		<button
			onclick={createKey}
			disabled={creating || !newKeyName.trim()}
			class="px-4 py-2 bg-blue-600 text-white rounded-md text-sm font-medium hover:bg-blue-700 disabled:opacity-50"
		>
			{creating ? $t('common.creating') : $t('settings.createKey')}
		</button>
	</div>

	<!-- Show newly created key -->
	{#if showKey}
		<div class="mb-6 p-4 bg-yellow-50 border border-yellow-200 rounded-md">
			<div class="text-sm font-medium text-yellow-800 mb-2">{$t('settings.keyPairWarning')}</div>
			{#if showPublicKey}
				<div class="mb-2">
					<div class="text-xs font-medium text-yellow-700 mb-1">{$t('settings.publicKey')}</div>
					<div class="flex items-center gap-2">
						<code class="flex-1 text-sm bg-white px-3 py-2 rounded border border-yellow-300 font-mono break-all">{showPublicKey}</code>
						<button onclick={() => copyKey(showPublicKey!)} class="px-3 py-2 text-sm bg-yellow-100 hover:bg-yellow-200 rounded">{$t('common.copy')}</button>
					</div>
				</div>
			{/if}
			<div class="mb-2">
				<div class="text-xs font-medium text-yellow-700 mb-1">{$t('settings.secretKey')}</div>
				<div class="flex items-center gap-2">
					<code class="flex-1 text-sm bg-white px-3 py-2 rounded border border-yellow-300 font-mono break-all">{showKey}</code>
					<button onclick={() => copyKey(showKey!)} class="px-3 py-2 text-sm bg-yellow-100 hover:bg-yellow-200 rounded">{$t('common.copy')}</button>
				</div>
			</div>
			<button onclick={() => { showKey = null; showPublicKey = null; }} class="mt-2 px-3 py-2 text-sm text-gray-500 hover:text-gray-700">{$t('common.dismiss')}</button>
		</div>
	{/if}

	{#if loading}
		<div class="text-center py-8 text-gray-500">{$t('settings.loadingKeys')}</div>
	{:else if !$currentProject}
		<div class="text-center py-8 text-gray-500">{$t('common.noProject')}.</div>
	{:else if keys.length === 0}
		<div class="text-center py-8 text-gray-500">{$t('settings.noKeys')}</div>
	{:else}
		<table class="min-w-full divide-y divide-gray-200">
			<thead class="bg-gray-50">
				<tr>
					<th class="px-4 py-3 text-left text-xs font-medium text-gray-500 uppercase">{$t('common.name')}</th>
					<th class="px-4 py-3 text-left text-xs font-medium text-gray-500 uppercase">{$t('settings.apiKeys')}</th>
					<th class="px-4 py-3 text-left text-xs font-medium text-gray-500 uppercase">{$t('common.created')}</th>
					<th class="px-4 py-3 text-right text-xs font-medium text-gray-500 uppercase">{$t('common.actions')}</th>
				</tr>
			</thead>
			<tbody class="divide-y divide-gray-200">
				{#each keys as key}
					<tr>
						<td class="px-4 py-3 text-sm font-medium text-gray-900">{key.name || $t('common.unnamed')}</td>
						<td class="px-4 py-3 text-sm text-gray-500 font-mono">{(key.display_secret_key || key.key || '').slice(0, 8)}...{(key.display_secret_key || key.key || '').slice(-4)}</td>
						<td class="px-4 py-3 text-sm text-gray-500">{new Date(key.created_at).toLocaleDateString()}</td>
						<td class="px-4 py-3 text-right">
							{#if key.key}
								<button onclick={() => copyKey(key.key)} class="text-sm text-blue-600 hover:text-blue-800 mr-3">{$t('common.copy')}</button>
							{/if}
							<button onclick={() => deleteKey(key.id)} class="text-sm text-red-600 hover:text-red-800">{$t('common.delete')}</button>
						</td>
					</tr>
				{/each}
			</tbody>
		</table>
	{/if}
</div>
