-- Support tickets have been retired. Remove their history and auxiliary state.
-- Resolve hashed mail receipts while message IDs and owners are still available.
DO $$
BEGIN
    IF to_regclass('support_ticket_messages') IS NOT NULL
       AND to_regclass('support_tickets') IS NOT NULL THEN
        DELETE FROM settings
        WHERE key IN (
            SELECT 'notification_email_delivery:v2:' || encode(sha256(
                convert_to('ticket.reply', 'UTF8') || decode('00', 'hex') ||
                convert_to('support_ticket_message', 'UTF8') || decode('00', 'hex') ||
                convert_to(m.id::text, 'UTF8') || decode('00', 'hex') ||
                convert_to(lower(btrim(u.email)), 'UTF8') || decode('00', 'hex')
            ), 'hex')
            FROM support_ticket_messages AS m
            JOIN support_tickets AS t ON t.id = m.ticket_id
            JOIN users AS u ON u.id = t.user_id
        );
    END IF;
END;
$$;

-- Drop children first; avoid CASCADE so unrelated dependencies are never removed.
DROP TABLE IF EXISTS support_ticket_views;
DROP TABLE IF EXISTS support_ticket_messages;
DROP TABLE IF EXISTS support_ticket_rate_limits;
DROP TABLE IF EXISTS support_ticket_user_locks;
DROP TABLE IF EXISTS support_tickets;

-- Only remove settings owned by the retired feature, keeping shared mail state.
DELETE FROM settings
WHERE starts_with(key, 'ticket_reply_email:')
   OR starts_with(key, 'notification_email_template:ticket.reply:')
   OR starts_with(key, 'notification_email_preference:ticket.reply:')
   OR starts_with(key, 'notification_email_delivery:ticket.reply:');
