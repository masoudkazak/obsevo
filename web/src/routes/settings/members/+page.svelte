<script lang="ts">
	import { api, type Member, type Organization } from '$lib/api';
	import { organizations, notifications } from '$lib/stores';

	let members = $state<Member[]>([]);
	let loading = $state(true);
	let selectedOrg = $state<Organization | null>(null);

	async function loadMembers() {
		if (!selectedOrg) return;
		loading = true;
		try {
			members = await api.settings.members.list(selectedOrg.id);
		} catch (e: any) {
			notifications.error(e.message || 'Failed to load members');
		} finally {
			loading = false;
		}
	}

	async function updateRole(userId: string, role: string) {
		if (!selectedOrg) return;
		try {
			await api.settings.members.updateRole(userId, selectedOrg.id, role);
			await loadMembers();
			notifications.success('Role updated');
		} catch (e: any) {
			notifications.error(e.message || 'Failed to update role');
		}
	}

	async function removeMember(userId: string) {
		if (!selectedOrg) return;
		if (!confirm('Are you sure you want to remove this member?')) return;
		try {
			await api.settings.members.remove(userId, selectedOrg.id);
			await loadMembers();
			notifications.success('Member removed');
		} catch (e: any) {
			notifications.error(e.message || 'Failed to remove member');
		}
	}

	function roleBadge(role: string): string {
		switch (role) {
			case 'ADMIN': return 'bg-red-100 text-red-800';
			case 'EDITOR': return 'bg-blue-100 text-blue-800';
			default: return 'bg-gray-100 text-gray-800';
		}
	}

	$effect(() => {
		if ($organizations.length > 0 && !selectedOrg) {
			selectedOrg = $organizations[0];
		}
	});

	$effect(() => {
		if (selectedOrg) {
			loadMembers();
		}
	});
</script>

<svelte:head>
	<title>Members — Settings — Langfuse Light</title>
</svelte:head>

<div class="bg-white rounded-lg border border-gray-200 p-6">
	<div class="flex items-center justify-between mb-4">
		<h2 class="text-lg font-semibold text-gray-900">Members</h2>
		{#if $organizations.length > 1}
			<select
				bind:value={selectedOrg}
				class="px-3 py-2 border border-gray-300 rounded-md text-sm focus:outline-none focus:ring-2 focus:ring-blue-500"
			>
				{#each $organizations as org}
					<option value={org}>{org.name}</option>
				{/each}
			</select>
		{/if}
	</div>

	{#if loading}
		<div class="text-center py-8 text-gray-500">Loading members...</div>
	{:else if members.length === 0}
		<div class="text-center py-8 text-gray-500">No members found.</div>
	{:else}
		<table class="min-w-full divide-y divide-gray-200">
			<thead class="bg-gray-50">
				<tr>
					<th class="px-4 py-3 text-left text-xs font-medium text-gray-500 uppercase">User</th>
					<th class="px-4 py-3 text-left text-xs font-medium text-gray-500 uppercase">Email</th>
					<th class="px-4 py-3 text-left text-xs font-medium text-gray-500 uppercase">Role</th>
					<th class="px-4 py-3 text-right text-xs font-medium text-gray-500 uppercase">Actions</th>
				</tr>
			</thead>
			<tbody class="divide-y divide-gray-200">
				{#each members as member}
					<tr>
						<td class="px-4 py-3 text-sm font-medium text-gray-900">{member.user_name || '—'}</td>
						<td class="px-4 py-3 text-sm text-gray-500">{member.email}</td>
						<td class="px-4 py-3 text-sm">
							<select
								value={member.role}
								onchange={(e) => updateRole(member.user_id, (e.target as HTMLSelectElement).value)}
								class="text-xs font-medium px-2 py-1 rounded {roleBadge(member.role)}"
							>
								<option value="VIEWER">VIEWER</option>
								<option value="EDITOR">EDITOR</option>
								<option value="ADMIN">ADMIN</option>
							</select>
						</td>
						<td class="px-4 py-3 text-right">
							<button onclick={() => removeMember(member.user_id)} class="text-sm text-red-600 hover:text-red-800">Remove</button>
						</td>
					</tr>
				{/each}
			</tbody>
		</table>
	{/if}
</div>
