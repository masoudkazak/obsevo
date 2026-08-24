<script lang="ts">
	import { goto } from '$app/navigation';
	import { isAuthenticated } from '$lib/stores';

	$effect(() => {
		if ($isAuthenticated) {
			goto('/traces');
		} else {
			goto('/login');
		}
	});
</script>

<svelte:head>
	<title>Langfuse Light</title>
</svelte:head>

<div class="flex items-center justify-center min-h-screen">
	<p class="text-gray-500">Redirecting...</p>
</div>
