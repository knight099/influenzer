import os
import json
import psycopg2
from urllib.parse import urlparse

DATABASE_URL = "postgresql://neondb_owner:npg_uIcM5kWHUpa2@ep-ancient-wave-a1tlq6o3-pooler.ap-southeast-1.aws.neon.tech/neondb?sslmode=require"

def main():
    try:
        conn = psycopg2.connect(DATABASE_URL)
        cur = conn.cursor()
        
        # 1. Fetch Users
        cur.execute("SELECT id, email FROM users;")
        users = cur.fetchall()
        
        print(f"Discovered {len(users)} users.")
        
        for user_id, email in users:
            print(f"Injecting Analytics for: {email}")
            
            # 2. Assign Creator Role to ensure they map
            cur.execute("UPDATE users SET role = 'CREATOR' WHERE id = %s", (user_id,))
            
            cached_stats = {
                "instagram": {
                    "id": "100010001",
                    "username": "vaibhawkrishna",
                    "name": "Vaibhaw Krishna",
                    "biography": "Tech Enthusiast | Content Creator 💻",
                    "followers_count": 84500,
                    "follows_count": 120,
                    "media_count": 420,
                    "profile_picture": "https://images.unsplash.com/photo-1599566150163-29194dcaad36?q=80&w=200&auto=format&fit=crop"
                },
                "youtube": {
                    "subscriber_count": "145000",
                    "view_count": "2400500",
                    "video_count": "158",
                    "channel_title": "Vaibhaw Tech Insights",
                    "channel_url": "vaibhaw-tech",
                    "thumbnail": "https://images.unsplash.com/photo-1511367461989-f85a21fda167?q=80&w=200&auto=format&fit=crop",
                    "published_at": "2020-05-10T14:00:00Z",
                    "country": "IN"
                }
            }
            
            cached_stats_json = json.dumps(cached_stats)
            
            cur.execute("""
                INSERT INTO creator_profiles (user_id, cached_stats, min_budget, city) 
                VALUES (%s, %s, 10000, 'Bangalore')
                ON CONFLICT (user_id) 
                DO UPDATE SET cached_stats = EXCLUDED.cached_stats;
            """, (user_id, cached_stats_json))
            
        conn.commit()
        cur.close()
        conn.close()
        print("Successfully Injected Mock Stats!")
        
    except Exception as e:
        print(f"Import Failed: {e}")

if __name__ == "__main__":
    main()
