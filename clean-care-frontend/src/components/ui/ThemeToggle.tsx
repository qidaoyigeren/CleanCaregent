import { useAppStore, type ThemeMode } from '../../store/appStore';

const THEMES: ThemeMode[] = ['light', 'dark', 'system'];

export default function ThemeToggle() {
  const theme = useAppStore((state): ThemeMode => state.theme ?? 'system');
  const setTheme = useAppStore((state) => state.setTheme);

  const cycleTheme = () => {
    const currentIndex = THEMES.indexOf(theme);
    const nextIndex = (currentIndex + 1) % THEMES.length;
    const nextTheme = THEMES[nextIndex] as ThemeMode;
    setTheme(nextTheme);
  };

  const getIcon = () => {
    switch (theme) {
      case 'light':
        return '☀️';
      case 'dark':
        return '🌙';
      case 'system':
        return '💻';
      default:
        return '💻';
    }
  };

  const getLabel = () => {
    switch (theme) {
      case 'light':
        return '浅色模式';
      case 'dark':
        return '深色模式';
      case 'system':
        return '跟随系统';
      default:
        return '跟随系统';
    }
  };

  return (
    <button
      className="theme-toggle"
      onClick={cycleTheme}
      title={getLabel()}
      aria-label={getLabel()}
    >
      <span className="theme-toggle__icon">{getIcon()}</span>
    </button>
  );
}
