/** @type {import('tailwindcss').Config} */
export default {
  content: ['./index.html', './src/**/*.{vue,js,ts,jsx,tsx}'],
  darkMode: 'class',
  theme: {
    extend: {
      colors: {
        // Blue accents shared by public pages and console controls
        primary: {
          50: '#eff4ff',
          100: '#dde8ff',
          200: '#c0d4ff',
          300: '#9bbbff',
          400: '#729cfb',
          500: '#3468e8',
          600: '#2858d6',
          700: '#2149b8',
          800: '#203f97',
          900: '#1f3878',
          950: '#14244f'
        },
        // 辅助色 - 中性冷灰
        accent: {
          50: '#f6f7f9',
          100: '#eceef2',
          200: '#dcdfe6',
          300: '#c5cad3',
          400: '#a4abb8',
          500: '#808898',
          600: '#505767',
          700: '#313640',
          800: '#1d2027',
          900: '#15171c',
          950: '#0e1014'
        },
        // 深色模式背景
        dark: {
          50: '#f6f7f9',
          100: '#eceef2',
          200: '#dcdfe6',
          300: '#c5cad3',
          400: '#a4abb8',
          500: '#808898',
          600: '#505767',
          700: '#313640',
          800: '#1d2027',
          900: '#15171c',
          950: '#0e1014'
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
        glow: '0 0 20px rgba(40, 88, 214, 0.08)',
        'glow-lg': '0 0 40px rgba(40, 88, 214, 0.10)',
        card: '0 1px 3px rgba(0, 0, 0, 0.04), 0 1px 2px rgba(0, 0, 0, 0.06)',
        'card-hover': '0 10px 40px rgba(0, 0, 0, 0.08)',
        'inner-glow': 'inset 0 1px 0 rgba(255, 255, 255, 0.1)'
      },
      backgroundImage: {
        'gradient-radial': 'radial-gradient(var(--tw-gradient-stops))',
        'gradient-primary': 'linear-gradient(135deg, #3468e8 0%, #2858d6 55%, #2149b8 100%)',
        'gradient-dark': 'linear-gradient(135deg, #272b34 0%, #15171c 100%)',
        'gradient-glass':
          'linear-gradient(135deg, rgba(255,255,255,0.1) 0%, rgba(255,255,255,0.05) 100%)',
        'mesh-gradient':
          'radial-gradient(at 40% 20%, rgba(40, 88, 214, 0.04) 0px, transparent 50%), radial-gradient(at 80% 0%, rgba(10, 122, 144, 0.03) 0px, transparent 50%)'
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
          '0%': { boxShadow: '0 0 20px rgba(40, 88, 214, 0.10)' },
          '100%': { boxShadow: '0 0 30px rgba(40, 88, 214, 0.16)' }
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
