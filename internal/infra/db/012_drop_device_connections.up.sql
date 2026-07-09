-- Учёт устройств переведён на нативный HWID-механизм Remnawave
-- (панель считает устройства по x-hwid при запросе подписки), поэтому
-- локальная таблица device_connections больше не нужна.
DROP TABLE IF EXISTS device_connections;
