-- Migración: Añadir columna freq_range a la tabla estaciones
-- El campo es opcional (nullable). Si no se define, el backend devuelve "VHF/UHF Standard".
-- Ejecutar en: psql -d sentinel_sso -f add_freq_range_to_estaciones.sql

ALTER TABLE estaciones
    ADD COLUMN IF NOT EXISTS freq_range VARCHAR(50) DEFAULT NULL;

-- Ejemplos de actualización para estaciones existentes:
-- UPDATE estaciones SET freq_range = '80MHz-120MHz' WHERE id = 'MALAGA_01';
-- UPDATE estaciones SET freq_range = '140MHz-160MHz' WHERE id = 'MIJAS_VHF';

COMMENT ON COLUMN estaciones.freq_range IS 'Rango de frecuencia visible para el operador en el selector AWACS. Ej: "80MHz-120MHz". Nullable: si es NULL el backend usa "VHF/UHF Standard".';
