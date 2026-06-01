import axios, { AxiosError } from "axios";
import { getAccessToken, notifyUnauthorized } from "@/shared/auth/tokenStore";
import { env } from "@/shared/config/env";

export class ApiError extends Error {
  status?: number;

  constructor(message: string, status?: number) {
    super(message);
    this.name = "ApiError";
    this.status = status;
  }
}

export const apiClient = axios.create({
  baseURL: env.apiBaseUrl,
  timeout: 8000,
});

apiClient.interceptors.request.use((config) => {
  const token = getAccessToken();
  if (token) {
    config.headers.Authorization = `Bearer ${token}`;
  }
  return config;
});

apiClient.interceptors.response.use(
  (response) => response,
  (error) => {
    if (error instanceof AxiosError && error.response?.status === 401) {
      notifyUnauthorized();
    }
    return Promise.reject(error);
  },
);

export async function getJson<T>(path: string): Promise<T> {
  try {
    const response = await apiClient.get<T>(path);
    return response.data;
  } catch (error) {
    throw toApiError(error);
  }
}

export async function postJson<TResponse, TBody>(
  path: string,
  body: TBody,
): Promise<TResponse> {
  try {
    const response = await apiClient.post<TResponse>(path, body);
    return response.data;
  } catch (error) {
    throw toApiError(error);
  }
}

export async function patchJson<TResponse, TBody>(
  path: string,
  body: TBody,
): Promise<TResponse> {
  try {
    const response = await apiClient.patch<TResponse>(path, body);
    return response.data;
  } catch (error) {
    throw toApiError(error);
  }
}

export async function putJson<TResponse, TBody>(
  path: string,
  body: TBody,
): Promise<TResponse> {
  try {
    const response = await apiClient.put<TResponse>(path, body);
    return response.data;
  } catch (error) {
    throw toApiError(error);
  }
}

export async function deleteJson<TResponse>(path: string): Promise<TResponse> {
  try {
    const response = await apiClient.delete<TResponse>(path);
    return response.data;
  } catch (error) {
    throw toApiError(error);
  }
}

export function toApiError(error: unknown): ApiError {
  if (error instanceof AxiosError) {
    const detail = error.response?.data;
    const message =
      typeof detail === "object" && detail && "error" in detail
        ? String(detail.error)
        : error.message;
    return new ApiError(message, error.response?.status);
  }

  if (error instanceof Error) {
    return new ApiError(error.message);
  }

  return new ApiError("unknown api error");
}
