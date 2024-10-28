import i18n from 'i18next';
import { initReactI18next } from 'react-i18next';
import LanguageDetector from 'i18next-browser-languagedetector';

import en from '../locales/en/translation.json';
import es from '../locales/es/translation.json';
import fr from '../locales/fr/translation.json';
import de from '../locales/de/translation.json';
import ru from '../locales/ru/translation.json';
import zhHans from '../locales/zh-CN/translation.json';
import zhHant from '../locales/zh-HK/translation.json';

i18n
    .use(LanguageDetector)
    .use(initReactI18next)
    .init({
        resources: {
            en: { translation: en },
            es: { translation: es },
            fr: { translation: fr },
            de: { translation: de },
            ru: { translation: ru },
            // Chinese (Simplified)
            'zh-CN': { translation: zhHans },
            // Chinese (Traditional)
            'zh-HK': { translation: zhHant },
        },
        fallbackLng: 'en',
        interpolation: {
            escapeValue: false
        }
    });

export { i18n };