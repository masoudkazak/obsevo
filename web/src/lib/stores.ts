import { writable, derived } from 'svelte/store';
import type { User, Project, Organization } from './api';

function createAuthStore() {
	const stored = typeof localStorage !== 'undefined' ? localStorage.getItem('token') : null;
	const storedUser = typeof localStorage !== 'undefined' ? localStorage.getItem('user') : null;

	const { subscribe, set, update } = writable<{
		token: string | null;
		user: User | null;
	}>({
		token: stored,
		user: storedUser ? JSON.parse(storedUser) : null
	});

	return {
		subscribe,
		login: (token: string, user: User) => {
			localStorage.setItem('token', token);
			localStorage.setItem('user', JSON.stringify(user));
			set({ token, user });
		},
		logout: () => {
			localStorage.removeItem('token');
			localStorage.removeItem('user');
			localStorage.removeItem('current_project');
			localStorage.removeItem('project_id');
			set({ token: null, user: null });
		}
	};
}

export const auth = createAuthStore();

export const isAuthenticated = derived(auth, ($auth) => !!$auth.token);

export const projects = writable<Project[]>([]);

/**
 * The project every scoped API call is made against.
 *
 * The selected id is mirrored into localStorage so `api.ts` can attach the
 * `X-Project-Id` header to every request without importing this module, which
 * would be a circular dependency.
 */
function createCurrentProjectStore() {
	const stored =
		typeof localStorage !== 'undefined' ? localStorage.getItem('current_project') : null;

	const { subscribe, set } = writable<Project | null>(stored ? JSON.parse(stored) : null);

	return {
		subscribe,
		set: (project: Project | null) => {
			if (typeof localStorage !== 'undefined') {
				if (project) {
					localStorage.setItem('current_project', JSON.stringify(project));
					localStorage.setItem('project_id', project.id);
				} else {
					localStorage.removeItem('current_project');
					localStorage.removeItem('project_id');
				}
			}
			set(project);
		}
	};
}

export const currentProject = createCurrentProjectStore();

export const organizations = writable<Organization[]>([]);

function createNotificationStore() {
	const { subscribe, update } = writable<Array<{ id: number; type: string; message: string }>>([]);
	let nextId = 0;

	return {
		subscribe,
		success: (message: string) => {
			const id = nextId++;
			update((n) => [...n, { id, type: 'success', message }]);
			setTimeout(() => update((n) => n.filter((x) => x.id !== id)), 3000);
		},
		error: (message: string) => {
			const id = nextId++;
			update((n) => [...n, { id, type: 'error', message }]);
			setTimeout(() => update((n) => n.filter((x) => x.id !== id)), 5000);
		}
	};
}

export const notifications = createNotificationStore();
