import { mount } from 'svelte';
import '@fontsource-variable/dm-sans';
import '@fontsource/jetbrains-mono/400.css';
import '@fontsource/jetbrains-mono/500.css';
import App from './App.svelte';
import './styles.css';

mount(App, { target: document.getElementById('app')! });
