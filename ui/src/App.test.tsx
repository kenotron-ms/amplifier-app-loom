import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, it, expect } from 'vitest'
import App from './App'

describe('App hub navigation', () => {
  it('renders all three tabs', () => {
    render(<App />)
    expect(screen.getByRole('tab', { name: 'Projects' })).toBeInTheDocument()
    expect(screen.getByRole('tab', { name: 'Jobs' })).toBeInTheDocument()
    expect(screen.getByRole('tab', { name: 'Mirror' })).toBeInTheDocument()
  })

  it('defaults to Projects tab', () => {
    render(<App />)
    expect(screen.getByRole('tab', { name: 'Projects' })).toHaveAttribute('aria-selected', 'true')
  })

  it('switches to Jobs tab on click', async () => {
    render(<App />)
    await userEvent.click(screen.getByRole('tab', { name: 'Jobs' }))
    expect(screen.getByRole('tab', { name: 'Jobs' })).toHaveAttribute('aria-selected', 'true')
  })
})
