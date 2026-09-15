<script lang="ts">
	import { t } from '$lib/i18n';

	let { children } = $props();
</script>

<div class="max-w-4xl mx-auto">
	<h1 class="text-2xl font-bold text-gray-900 mb-6">{$t('settings.heading')}</h1>
	<div class="flex gap-4 mb-6 border-b border-gray-200">
		<a href="/settings" class="px-3 py-2 text-sm font-medium border-b-2 border-transparent text-gray-500 hover:text-gray-700">{$t('settings.profile')}</a>
		<a href="/settings/api-keys" class="px-3 py-2 text-sm font-medium border-b-2 border-transparent text-gray-500 hover:text-gray-700">{$t('settings.apiKeys')}</a>
		<a href="/settings/members" class="px-3 py-2 text-sm font-medium border-b-2 border-transparent text-gray-500 hover:text-gray-700">{$t('settings.members')}</a>
	</div>
	{@render children()}
</div>
