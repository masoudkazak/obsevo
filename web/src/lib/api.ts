const BASE = '';

async function request<T>(path: string, options: RequestInit = {}): Promise<T> {
	const token = typeof localStorage !== 'undefined' ? localStorage.getItem('token') : null;
	const headers: Record<string, string> = {
		'Content-Type': 'application/json',
		...(options.headers as Record<string, string> || {})
	};
	if (token) {
		headers['Authorization'] = `Bearer ${token}`;
	}
	const res = await fetch(`${BASE}${path}`, { ...options, headers });
	if (!res.ok) {
		const body = await res.json().catch(() => ({ error: res.statusText }));
		throw new Error(body.error || `HTTP ${res.status}`);
	}
	return res.json();
}

export interface User {
	id: string;
	email: string;
	name: string;
}

export interface AuthResponse {
	token: string;
	user: User;
}

export interface Organization {
	id: string;
	name: string;
	created_at: string;
}

export interface Project {
	id: string;
	name: string;
	org_id: string;
	created_at: string;
}

export interface Trace {
	id: string;
	project_id: string;
	name: string | null;
	input: unknown;
	output: unknown;
	metadata: unknown;
	user_id: string | null;
	session_id: string | null;
	tags: string[];
	start_time: string | null;
	end_time: string | null;
	total_cost: number | null;
	token_usage: unknown;
	created_at: string;
}

export interface TraceDetail extends Trace {
	project_name: string;
}

export interface Observation {
	id: string;
	trace_id: string;
	type: string;
	name: string | null;
	input: unknown;
	output: unknown;
	metadata: unknown;
	model: string | null;
	model_parameters: unknown;
	start_time: string | null;
	end_time: string | null;
	token_usage: unknown;
	cost: number | null;
	status: string;
	parent_observation_id: string | null;
}

export interface TraceWithObservations {
	trace: TraceDetail;
	observations: Observation[];
}

export interface Prompt {
	id: string;
	project_id: string;
	name: string;
	version: number;
	prompt: string;
	config: unknown;
	is_active: boolean;
	created_at: string;
}

export interface PromptWithVariables extends Prompt {
	compiled_template: string;
	variables: string[];
}

export interface TraceListResponse {
	traces: Trace[];
	total: number;
}

export interface PromptListResponse {
	prompts: Prompt[];
}

export interface PromptVersionsResponse {
	versions: Prompt[];
}

export interface IngestionResponse {
	traces_created: number;
	observations_created: number;
}

export const api = {
	auth: {
		register: (data: { email: string; password: string; name?: string }) =>
			request<AuthResponse>('/api/auth/register', { method: 'POST', body: JSON.stringify(data) }),
		login: (data: { email: string; password: string }) =>
			request<AuthResponse>('/api/auth/login', { method: 'POST', body: JSON.stringify(data) })
	},
	organizations: {
		list: () => request<Organization[]>('/api/organizations'),
		create: (data: { name: string }) =>
			request<Organization>('/api/organizations', { method: 'POST', body: JSON.stringify(data) })
	},
	projects: {
		list: () => request<Project[]>('/api/projects'),
		create: (data: { name: string }) =>
			request<Project>('/api/projects', { method: 'POST', body: JSON.stringify(data) })
	},
	traces: {
		list: (projectId: string, params?: { name?: string; limit?: number; offset?: number }) => {
			const q = new URLSearchParams({ project_id: projectId });
			if (params?.name) q.set('name', params.name);
			if (params?.limit) q.set('limit', String(params.limit));
			if (params?.offset) q.set('offset', String(params.offset));
			return request<TraceListResponse>(`/api/traces?${q}`);
		},
		get: (id: string) => request<TraceWithObservations>(`/api/traces/${id}`),
		create: (data: Partial<Trace> & { id: string }, projectId: string) =>
			request<Trace>(`/api/traces?project_id=${projectId}`, { method: 'POST', body: JSON.stringify(data) })
	},
	observations: {
		get: (id: string) => request<Observation>(`/api/observations/${id}`)
	},
	prompts: {
		list: (projectId: string) => {
			const q = new URLSearchParams({ project_id: projectId });
			return request<PromptListResponse>(`/api/prompts?${q}`);
		},
		getByName: (name: string, projectId: string) => {
			const q = new URLSearchParams({ project_id: projectId });
			return request<PromptWithVariables>(`/api/prompts/${encodeURIComponent(name)}?${q}`);
		},
		create: (data: { name: string; prompt: string; config?: unknown }, projectId: string) => {
			const q = new URLSearchParams({ project_id: projectId });
			return request<Prompt>(`/api/prompts?${q}`, { method: 'POST', body: JSON.stringify(data) });
		},
		update: (name: string, data: { prompt: string; config?: unknown; is_active?: boolean }, projectId: string) => {
			const q = new URLSearchParams({ project_id: projectId });
			return request<Prompt>(`/api/prompts/${encodeURIComponent(name)}?${q}`, { method: 'PUT', body: JSON.stringify(data) });
		},
		versions: (name: string, projectId: string) => {
			const q = new URLSearchParams({ project_id: projectId });
			return request<PromptVersionsResponse>(`/api/prompts/${encodeURIComponent(name)}/versions?${q}`);
		},
		setActive: (name: string, version: number, projectId: string) => {
			const q = new URLSearchParams({ project_id: projectId });
			return request<{ status: string }>(`/api/prompts/${encodeURIComponent(name)}/active?${q}`, {
				method: 'POST',
				body: JSON.stringify({ version })
			});
		}
	}
};
