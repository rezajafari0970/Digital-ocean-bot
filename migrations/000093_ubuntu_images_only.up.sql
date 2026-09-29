UPDATE accounts
SET preferred_images='["ubuntu-26-04-x64","ubuntu-22-04-x64","ubuntu-24-04-x64"]'::jsonb,
    preferred_image='ubuntu-26-04-x64',
    updated_at=now();
