<script lang="ts">
	import '../app.css';
	import { auth, isAuthenticated, projects, currentProject, notifications } from '$lib/stores';
	import { api } from '$lib/api';
	import { goto } from '$app/navigation';
	import { page } from '$app/state';

	let { children } = $props();
	let sidebarOpen = $state(true);

	$effect(() => {
		if (!$isAuthenticated) {
			goto('/login');
		}
	});

	$effect(() => {
		if ($isAuthenticated && $projects.length === 0) {
			api.projects.list().then((p) => {
				projects.set(p);
				if (p.length > 0 && !$currentProject) {
					currentProject.set(p[0]);
				}
			}).catch(() => {});
		}
	});

	function handleLogout() {
		auth.logout();
		goto('/login');
	}

	function selectProject(project: typeof $currentProject) {
		currentProject.set(project);
	}

	let currentPath = $derived(page.url.pathname);
</script>

<svelte:head>
	<link rel="icon" href="/favicon.svg" />
	<title>Langfuse Light</title>
</svelte:head>

{#if $isAuthenticated}
	<div class="flex h-screen bg-gray-50">
		<!-- Sidebar -->
		<aside class="{sidebarOpen ? 'w-64' : 'w-16'} bg-gray-900 text-white flex flex-col transition-all duration-200">
			<div class="flex items-center justify-between p-4 border-b border-gray-700">
				{#if sidebarOpen}
					<span class="text-lg font-bold tracking-tight">Langfuse Light</span>
				{/if}
				<button
					onclick={() => sidebarOpen = !sidebarOpen}
					class="p-1 rounded hover:bg-gray-700 text-gray-400"
				>
					{#if sidebarOpen}
						<svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
							<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M11 19l-7-7 7-7m8 14l-7-7 7-7" />
						</svg>
					{:else}
						<svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
							<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M13 5l7 7-7 7M5 5l7 7-7 7" />
						</svg>
					{/if}
				</button>
			</div>

			<!-- Project selector -->
			{#if sidebarOpen && $projects.length > 0}
				<div class="p-3 border-b border-gray-700">
					<label for="project-select" class="text-xs text-gray-400 uppercase tracking-wider">Project</label>
					<select
						id="project-select"
						class="w-full mt-1 bg-gray-800 border border-gray-600 rounded px-2 py-1.5 text-sm text-white focus:outline-none focus:ring-1 focus:ring-blue-500"
						onchange={(e) => {
							const p = $projects.find(p => p.id === (e.target as HTMLSelectElement).value);
							if (p) selectProject(p);
						}}
					>
						{#each $projects as project}
							<option value={project.id} selected={$currentProject?.id === project.id}>
								{project.name}
							</option>
						{/each}
					</select>
				</div>
			{/if}

			<nav class="flex-1 p-2 space-y-1">
				<a
					href="/traces"
					class="flex items-center gap-3 px-3 py-2 rounded text-sm font-medium
						{currentPath.startsWith('/traces') ? 'bg-gray-700 text-white' : 'text-gray-300 hover:bg-gray-800 hover:text-white'}"
				>
					<svg class="w-5 h-5 shrink-0" fill="none" stroke="currentColor" viewBox="0 0 24 24">
						<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 5H7a2 2 0 00-2 2v12a2 2 0 002 2h10a2 2 0 002-2V7a2 2 0 00-2-2h-2M9 5a2 2 0 002 2h2a2 2 0 002-2M9 5a2 2 0 012-2h2a2 2 0 012 2" />
					</svg>
					{#if sidebarOpen}Traces{/if}
				</a>
				<a
					href="/prompts"
					class="flex items-center gap-3 px-3 py-2 rounded text-sm font-medium
						{currentPath.startsWith('/prompts') ? 'bg-gray-700 text-white' : 'text-gray-300 hover:bg-gray-800 hover:text-white'}"
				>
					<svg class="w-5 h-5 shrink-0" fill="none" stroke="currentColor" viewBox="0 0 24 24">
						<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M11 5H6a2 2 0 00-2 2v11a2 2 0 002 2h11a2 2 0 002-2v-5m-1.414-9.414a2 2 0 112.828 2.828L11.828 15H9v-2.828l8.586-8.586z" />
					</svg>
					{#if sidebarOpen}Prompts{/if}
				</a>
			</nav>

			<!-- User section -->
			<div class="p-3 border-t border-gray-700">
				{#if sidebarOpen}
					<div class="text-sm text-gray-400 truncate mb-2">{$auth.user?.email}</div>
				{/if}
				<button
					onclick={handleLogout}
					class="flex items-center gap-2 w-full px-3 py-2 rounded text-sm text-gray-300 hover:bg-gray-800 hover:text-white"
				>
					<svg class="w-5 h-5 shrink-0" fill="none" stroke="currentColor" viewBox="0 0 24 24">
						<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M17 16l4-4m0 0l-4-4m4 4H7m6 4v1a3 3 0 01-3 3H6a3 3 0 01-3-3V7a3 3 0 013-3h4a3 3 0 013 3v1" />
					</svg>
					{#if sidebarOpen}Logout{/if}
				</button>
			</div>
		</aside>

		<!-- Main content -->
		<div class="flex-1 flex flex-col overflow-hidden">
			<main class="flex-1 overflow-y-auto p-6">
				{@render children()}
			</main>
		</div>
	</div>

	<!-- Notifications -->
	{#each $notifications as notification}
		<div
			class="fixed top-4 right-4 z-50 px-4 py-3 rounded shadow-lg text-sm font-medium
				{notification.type === 'success' ? 'bg-green-600 text-white' : 'bg-red-600 text-white'}"
		>
			{notification.message}
		</div>
	{/each}
{:else}
	{@render children()}
{/if}
