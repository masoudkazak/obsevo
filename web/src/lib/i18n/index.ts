import { register, init, getLocaleFromNavigator, locale } from 'svelte-i18n';
import en from './en.json';
import ru from './ru.json';

register('en', () => Promise.resolve({ default: en }));
register('ru', () => Promise.resolve({ default: ru }));

function getInitialLocale(): string {
	if (typeof localStorage !== 'undefined') {
		const stored = localStorage.getItem('locale');
		if (stored === 'en' || stored === 'ru') return stored;
	}
	return getLocaleFromNavigator()?.startsWith('ru') ? 'ru' : 'en';
}

export function setLocale(newLocale: string) {
	locale.set(newLocale);
	if (typeof localStorage !== 'undefined') {
		localStorage.setItem('locale', newLocale);
	}
}

export function getCurrentLocale(): string {
	return getInitialLocale();
}

init({
	fallbackLocale: 'en',
	initialLocale: getInitialLocale()
});

export { t } from 'svelte-i18n';
