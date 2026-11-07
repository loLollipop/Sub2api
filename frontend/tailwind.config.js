/** @type {import('tailwindcss').Config} */
export default {
  content: ['./index.html', './src/**/*.{vue,js,ts,jsx,tsx}'],
  darkMode: 'class',
  theme: {
    extend: {
      colors: {
        // Terracotta accents add warmth to public and console controls
        primary: {
          50: '#fcf4ee',
          100: '#f8e6d9',
          200: '#efceb7',
          300: '#efc6a8',
          400: '#e6ac86',
          500: '#ad5d37',
          600: '#a95e3c',
          700: '#8f4e32',
          800: '#75412c',
          900: '#603727',
          950: '#342119'
        },
        // 辅助色 - 中性暖灰
        accent: {
          50: '#faf9f7',
          100: '#f2efea',
          200: '#e6e0d8',
          300: '#d1c8bd',
          400: '#b3a89a',
          500: '#82786e',
          600: '#625a51',
          700: '#47423c',
          800: '#302d29',
          900: '#242321',
          950: '#1b1a18'
        },
        // 深色模式背景
        dark: {
          50: '#faf9f7',
          100: '#f0ece6',
          200: '#e1dad0',
          300: '#d1c8bd',
          400: '#bcb4a9',
          500: '#968c7f',
          600: '#746c61',
          700: '#45413b',
          800: '#2d2c29',
          900: '#242321',
          950: '#1b1a18'
        }
      },
      fontFamily: {
        sans: [
          'system-ui',
          '-apple-system',
          'BlinkMacSystemFont',
          'Segoe UI',
          'Roboto',
          'Helvetica Neue',
          'Arial',
          'PingFang SC',
          'Hiragino Sans GB',
          'Microsoft YaHei',
          'sans-serif'
        ],
        mono: ['ui-monospace', 'SFMono-Regular', 'Menlo', 'Monaco', 'Consolas', 'monospace']
      },
      boxShadow: {
        glass: '0 8px 32px rgba(0, 0, 0, 0.08)',
        'glass-sm': '0 4px 16px rgba(0, 0, 0, 0.06)',
        glow: '0 0 20px rgba(169, 94, 60, 0.08)',
        'glow-lg': '0 0 40px rgba(169, 94, 60, 0.10)',
        card: '0 1px 3px rgba(0, 0, 0, 0.04), 0 1px 2px rgba(0, 0, 0, 0.06)',
        'card-hover': '0 10px 40px rgba(0, 0, 0, 0.08)',
        'inner-glow': 'inset 0 1px 0 rgba(255, 255, 255, 0.1)'
      },
      backgroundImage: {
        'gradient-radial': 'radial-gradient(var(--tw-gradient-stops))',
        'gradient-primary': 'linear-gradient(135deg, #e6ac86 0%, #a95e3c 55%, #8f4e32 100%)',
        'gradient-dark': 'linear-gradient(135deg, #373530 0%, #242321 100%)',
        'gradient-glass':
          'linear-gradient(135deg, rgba(255,255,255,0.1) 0%, rgba(255,255,255,0.05) 100%)',
        'mesh-gradient':
          'radial-gradient(at 40% 20%, rgba(169, 94, 60, 0.04) 0px, transparent 50%), radial-gradient(at 80% 0%, rgba(167, 134, 78, 0.03) 0px, transparent 50%)'
      },
      animation: {
        'fade-in': 'fadeIn 0.3s ease-out',
        'slide-up': 'slideUp 0.3s ease-out',
        'slide-down': 'slideDown 0.3s ease-out',
        'slide-in-right': 'slideInRight 0.3s ease-out',
        'scale-in': 'scaleIn 0.2s ease-out',
        'pulse-slow': 'pulse 3s cubic-bezier(0.4, 0, 0.6, 1) infinite',
        shimmer: 'shimmer 2s linear infinite',
        glow: 'glow 2s ease-in-out infinite alternate'
      },
      keyframes: {
        fadeIn: {
          '0%': { opacity: '0' },
          '100%': { opacity: '1' }
        },
        slideUp: {
          '0%': { opacity: '0', transform: 'translateY(10px)' },
          '100%': { opacity: '1', transform: 'translateY(0)' }
        },
        slideDown: {
          '0%': { opacity: '0', transform: 'translateY(-10px)' },
          '100%': { opacity: '1', transform: 'translateY(0)' }
        },
        slideInRight: {
          '0%': { opacity: '0', transform: 'translateX(20px)' },
          '100%': { opacity: '1', transform: 'translateX(0)' }
        },
        scaleIn: {
          '0%': { opacity: '0', transform: 'scale(0.95)' },
          '100%': { opacity: '1', transform: 'scale(1)' }
        },
        shimmer: {
          '0%': { backgroundPosition: '-200% 0' },
          '100%': { backgroundPosition: '200% 0' }
        },
        glow: {
          '0%': { boxShadow: '0 0 20px rgba(169, 94, 60, 0.10)' },
          '100%': { boxShadow: '0 0 30px rgba(169, 94, 60, 0.16)' }
        }
      },
      backdropBlur: {
        xs: '2px'
      },
      borderRadius: {
        '4xl': '2rem'
      }
    }
  },
  plugins: []
}
