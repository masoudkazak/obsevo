<script lang="ts">
	import { t } from '$lib/i18n';
	import { api } from '$lib/api';
	import { auth } from '$lib/stores';
	import { goto } from '$app/navigation';

	let email = $state('');
	let password = $state('');
	let error = $state('');
	let loading = $state(false);

	async function handleLogin() {
		error = '';
		loading = true;
		try {
			const res = await api.auth.login({ email, password });
			auth.login(res.token, res.user);
			goto('/traces');
		} catch (e: any) {
			error = e.message || $t('auth.loginFailed');
		} finally {
			loading = false;
		}
	}
</script>

<svelte:head>
	<title>{$t('auth.loginTitleTab')}</title>
</svelte:head>

<div class="min-h-screen flex items-center justify-center bg-gray-50">
	<div class="w-full max-w-md p-8 bg-white rounded-lg shadow">
		<h1 class="text-2xl font-bold text-center mb-6">{$t('app.name')}</h1>
		<h2 class="text-lg text-gray-600 text-center mb-8">{$t('auth.loginTitle')}</h2>

		{#if error}
			<div class="mb-4 p-3 bg-red-50 border border-red-200 text-red-700 rounded text-sm">
				{error}
			</div>
		{/if}

		<form onsubmit={(e) => { e.preventDefault(); handleLogin(); }}>
			<div class="mb-4">
				<label for="email" class="block text-sm font-medium text-gray-700 mb-1">{$t('auth.email')}</label>
				<input
					id="email"
					type="email"
					bind:value={email}
					required
					class="w-full px-3 py-2 border border-gray-300 rounded-md shadow-sm focus:outline-none focus:ring-2 focus:ring-blue-500 focus:border-blue-500"
					placeholder="you@example.com"
				/>
			</div>
			<div class="mb-6">
				<label for="password" class="block text-sm font-medium text-gray-700 mb-1">{$t('auth.password')}</label>
				<input
					id="password"
					type="password"
					bind:value={password}
					required
					class="w-full px-3 py-2 border border-gray-300 rounded-md shadow-sm focus:outline-none focus:ring-2 focus:ring-blue-500 focus:border-blue-500"
					placeholder="••••••••"
				/>
			</div>
			<button
				type="submit"
				disabled={loading}
				class="w-full py-2 px-4 bg-blue-600 text-white rounded-md font-medium hover:bg-blue-700 focus:outline-none focus:ring-2 focus:ring-blue-500 focus:ring-offset-2 disabled:opacity-50"
			>
				{loading ? $t('auth.signingIn') : $t('auth.signIn')}
			</button>
		</form>

		<p class="mt-4 text-center text-sm text-gray-600">
			{$t('auth.noAccount')}
			<a href="/register" class="text-blue-600 hover:underline">{$t('auth.goRegister')}</a>
		</p>
	</div>
</div>
