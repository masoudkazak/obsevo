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

	// The API authorizes the caller against a project before any handler runs.
	// Sending the selected project on every request means detail endpoints
	// (a trace, an observation, a score) are scoped without each call site
	// having to thread the id through.
	const projectId =
		typeof localStorage !== 'undefined' ? localStorage.getItem('project_id') : null;
	if (projectId && !headers['X-Project-Id']) {
		headers['X-Project-Id'] = projectId;
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

export interface AnalyticsSummary {
	latency: LatencyStats;
	cost: CostStats;
	token_usage: TokenUsageStats;
	error_rate: ErrorRateStats;
	scores: ScoreAggregation[];
}

export interface LatencyStats {
	total_traces: number;
	avg_latency_seconds: number;
	min_latency_seconds: number;
	max_latency_seconds: number;
}

export interface CostStats {
	total_traces: number;
	total_cost: number;
	avg_cost: number;
	min_cost: number;
	max_cost: number;
}

export interface TokenUsageStats {
	total_traces: number;
	total_tokens: number;
	avg_tokens: number;
	total_input_tokens: number;
	total_output_tokens: number;
}

export interface ErrorRateStats {
	total_traces: number;
	error_traces: number;
	error_rate: number;
}

export interface ScoreAggregation {
	name: string;
	count: number;
	avg_value: number;
	min_value: number;
	max_value: number;
}

export interface CostOverTimePoint {
	time_bucket: string;
	trace_count: number;
	total_cost: number;
	avg_cost: number;
}

export interface LatencyOverTimePoint {
	time_bucket: string;
	trace_count: number;
	avg_latency_seconds: number;
	min_latency_seconds: number;
	max_latency_seconds: number;
}

export interface TokenUsageOverTimePoint {
	time_bucket: string;
	trace_count: number;
	total_tokens: number;
	total_input_tokens: number;
	total_output_tokens: number;
}

export interface TraceCountOverTimePoint {
	time_bucket: string;
	trace_count: number;
	error_count: number;
}

export interface APIKey {
	id: string;
	project_id: string;
	key: string;
	name: string;
	created_at: string;
	secret_key?: string;
	public_key?: string;
	display_secret_key?: string;
}

export interface Member {
	id: string;
	user_id: string;
	org_id: string;
	role: string;
	email: string;
	user_name: string;
}

export interface UserProfile {
	id: string;
	email: string;
	name: string;
	created_at: string;
}

export interface Dataset {
	id: string;
	project_id: string;
	name: string;
	description: string | null;
	created_at: string;
}

export interface DatasetItem {
	id: string;
	dataset_id: string;
	input: unknown;
	expected_output: unknown;
	metadata: unknown;
	source_trace_id: string | null;
	created_at: string;
}

export interface DatasetRun {
	id: string;
	dataset_id: string;
	name: string;
	created_at: string;
}

export interface DatasetRunItem {
	id: string;
	dataset_run_id: string;
	dataset_item_id: string;
	observation_id: string | null;
	score_id: string | null;
	created_at: string;
}

export interface RunResultExport {
	run_item_id: string;
	dataset_item_id: string;
	input: unknown;
	expected_output: unknown;
	observation_id: string;
	score_id: string;
	score?: { name: string; value: number } | null;
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
	},
	analytics: {
		summary: (projectId: string) => {
			const q = new URLSearchParams({ project_id: projectId });
			return request<AnalyticsSummary>(`/api/analytics?${q}`);
		},
		costOverTime: (projectId: string, days?: number) => {
			const q = new URLSearchParams({ project_id: projectId });
			if (days) q.set('days', String(days));
			return request<CostOverTimePoint[]>(`/api/analytics/cost-over-time?${q}`);
		},
		latencyOverTime: (projectId: string, days?: number) => {
			const q = new URLSearchParams({ project_id: projectId });
			if (days) q.set('days', String(days));
			return request<LatencyOverTimePoint[]>(`/api/analytics/latency-over-time?${q}`);
		},
		tokensOverTime: (projectId: string, days?: number) => {
			const q = new URLSearchParams({ project_id: projectId });
			if (days) q.set('days', String(days));
			return request<TokenUsageOverTimePoint[]>(`/api/analytics/tokens-over-time?${q}`);
		},
		tracesOverTime: (projectId: string, days?: number) => {
			const q = new URLSearchParams({ project_id: projectId });
			if (days) q.set('days', String(days));
			return request<TraceCountOverTimePoint[]>(`/api/analytics/traces-over-time?${q}`);
		}
	},
	settings: {
		profile: {
			get: () => request<UserProfile>('/api/profile'),
			update: (data: { name: string }) =>
				request<{ status: string }>('/api/profile', { method: 'PUT', body: JSON.stringify(data) })
		},
		apiKeys: {
			list: (projectId: string) => {
				const q = new URLSearchParams({ project_id: projectId });
				return request<APIKey[]>(`/api/api-keys?${q}`);
			},
			create: (projectId: string, data: { name: string }) => {
				const q = new URLSearchParams({ project_id: projectId });
				return request<APIKey>(`/api/api-keys?${q}`, { method: 'POST', body: JSON.stringify(data) });
			},
			delete: (keyId: string, projectId: string) => {
				const q = new URLSearchParams({ project_id: projectId });
				return request<{ status: string }>(`/api/api-keys/${keyId}?${q}`, { method: 'DELETE' });
			}
		},
		members: {
			list: (orgId: string) => {
				const q = new URLSearchParams({ org_id: orgId });
				return request<Member[]>(`/api/members?${q}`);
			},
			updateRole: (userId: string, orgId: string, role: string) => {
				const q = new URLSearchParams({ org_id: orgId });
				return request<{ status: string }>(`/api/members/${userId}/role?${q}`, {
					method: 'PUT',
					body: JSON.stringify({ role })
				});
			},
			remove: (userId: string, orgId: string) => {
				const q = new URLSearchParams({ org_id: orgId });
				return request<{ status: string }>(`/api/members/${userId}?${q}`, { method: 'DELETE' });
			}
		}
	},
	datasets: {
		list: (projectId: string) => {
			const q = new URLSearchParams({ project_id: projectId });
			return request<Dataset[]>(`/api/datasets?${q}`);
		},
		create: (projectId: string, data: { name: string; description?: string }) => {
			const q = new URLSearchParams({ project_id: projectId });
			return request<Dataset>(`/api/datasets?${q}`, { method: 'POST', body: JSON.stringify(data) });
		},
		get: (id: string) => request<Dataset>(`/api/datasets/${id}`),
		delete: (id: string) => request<{ status: string }>(`/api/datasets/${id}`, { method: 'DELETE' }),
		items: {
			list: (datasetId: string) => request<DatasetItem[]>(`/api/datasets/${datasetId}/items`),
			create: (datasetId: string, data: { input: unknown; expected_output?: unknown; metadata?: unknown; source_trace_id?: string }) =>
				request<DatasetItem>(`/api/datasets/${datasetId}/items`, { method: 'POST', body: JSON.stringify(data) })
		},
		importItems: (datasetId: string, data: unknown[] | string, type: 'json' | 'csv' = 'json') => {
			const headers: Record<string, string> = {};
			if (type === 'csv') {
				headers['Content-Type'] = 'text/csv';
			}
			return request<{ imported: number; items: DatasetItem[] }>(`/api/datasets/${datasetId}/import`, {
				method: 'POST',
				headers,
				body: type === 'csv' ? data as string : JSON.stringify(data)
			});
		},
		exportItems: (datasetId: string, format: 'json' | 'csv' = 'json') =>
			request<DatasetItem[]>(`/api/datasets/${datasetId}/export?format=${format}`),
		runs: {
			list: (datasetId: string) => request<DatasetRun[]>(`/api/datasets/${datasetId}/runs`),
			create: (datasetId: string, data: { name: string }) =>
				request<DatasetRun>(`/api/datasets/${datasetId}/runs`, { method: 'POST', body: JSON.stringify(data) }),
			get: (datasetId: string, runId: string) =>
				request<DatasetRun>(`/api/datasets/${datasetId}/runs/${runId}`),
			items: {
				list: (datasetId: string, runId: string) =>
					request<DatasetRunItem[]>(`/api/datasets/${datasetId}/runs/${runId}/items`),
				create: (datasetId: string, runId: string, data: { dataset_item_id: string; observation_id?: string; score_id?: string }) =>
					request<DatasetRunItem>(`/api/datasets/${datasetId}/runs/${runId}/items`, { method: 'POST', body: JSON.stringify(data) })
			},
			export: (datasetId: string, runId: string, format: 'json' | 'csv' = 'json') =>
				request<RunResultExport[]>(`/api/datasets/${datasetId}/runs/${runId}/export?format=${format}`)
		}
	}
};
