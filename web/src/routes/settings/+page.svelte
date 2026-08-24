<script lang="ts">
	import { api, type UserProfile } from '$lib/api';
	import { auth, notifications } from '$lib/stores';

	let profile = $state<UserProfile | null>(null);
	let name = $state('');
	let loading = $state(true);
	let saving = $state(false);

	async function loadProfile() {
		loading = true;
		try {
			profile = await api.settings.profile.get();
			name = profile.name || '';
		} catch (e: any) {
			notifications.error(e.message || 'Failed to load profile');
		} finally {
			loading = false;
		}
	}

	async function saveProfile() {
		saving = true;
		try {
			await api.settings.profile.update({ name });
			notifications.success('Profile updated');
		} catch (e: any) {
			notifications.error(e.message || 'Failed to update profile');
		} finally {
			saving = false;
		}
	}

	$effect(() => {
		loadProfile();
	});
</script>

<svelte:head>
	<title>Profile — Settings — Langfuse Light</title>
</svelte:head>

{#if loading}
	<div class="text-center py-8 text-gray-500">Loading profile...</div>
{:else}
	<div class="bg-white rounded-lg border border-gray-200 p-6">
		<h2 class="text-lg font-semibold text-gray-900 mb-4">Profile</h2>
		<div class="space-y-4">
			<div>
				<label for="email" class="block text-sm font-medium text-gray-700">Email</label>
				<input
					id="email"
					type="email"
					value={profile?.email ?? ''}
					disabled
					class="mt-1 block w-full px-3 py-2 border border-gray-300 rounded-md shadow-sm bg-gray-50 text-gray-500 text-sm"
				/>
			</div>
			<div>
				<label for="name" class="block text-sm font-medium text-gray-700">Name</label>
				<input
					id="name"
					type="text"
					bind:value={name}
					class="mt-1 block w-full px-3 py-2 border border-gray-300 rounded-md shadow-sm focus:outline-none focus:ring-2 focus:ring-blue-500 text-sm"
				/>
			</div>
			<div>
				<span class="block text-sm font-medium text-gray-700">Created</span>
				<div class="mt-1 text-sm text-gray-500">{profile?.created_at ? new Date(profile.created_at).toLocaleDateString() : '—'}</div>
			</div>
			<div class="pt-2">
				<button
					onclick={saveProfile}
					disabled={saving}
					class="px-4 py-2 bg-blue-600 text-white rounded-md text-sm font-medium hover:bg-blue-700 disabled:opacity-50"
				>
					{saving ? 'Saving...' : 'Save Changes'}
				</button>
			</div>
		</div>
	</div>
{/if}
