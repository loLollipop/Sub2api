import { apiClient } from './client'

export type LeaderboardPeriod = 'day' | 'week' | 'month' | 'year'

export interface LeaderboardModel {
  model: string
  total_tokens: number
  requests: number
  previous_tokens: number
  previous_requests: number
}

export interface LeaderboardResponse {
  period: LeaderboardPeriod
  timezone: string
  start: string
  end: string
  previous_start: string
  previous_end: string
  updated_at: string
  granularity: 'hour' | 'day' | 'month'
  total_tokens: number
  total_requests: number
  models: LeaderboardModel[]
  trend: Array<{ date: string; model: string; total_tokens: number }>
}

export async function getLeaderboard(period: LeaderboardPeriod, signal?: AbortSignal): Promise<LeaderboardResponse> {
  const { data } = await apiClient.get<LeaderboardResponse>('/leaderboard', { params: { period }, signal })
  return data
}
